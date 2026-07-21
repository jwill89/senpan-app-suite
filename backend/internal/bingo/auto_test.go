package bingo

import (
	"testing"

	"app-suite/internal/model"
)

// ── HalftimeThreshold ───────────────────────────────────────────────────────

func TestHalftimeThreshold(t *testing.T) {
	// A full five-column game keeps the classic 35-of-75 mark.
	if got := HalftimeThreshold([]model.BingoGamePattern{{PatternData: testPattern5x5()}}); got != 35 {
		t.Errorf("full-board threshold = %d; want 35", got)
	}
	// A postage-stamp game uses four columns (60 callable) → round(35/75*60) = 28.
	if got := HalftimeThreshold([]model.BingoGamePattern{{PatternData: postageStampPattern()}}); got != 28 {
		t.Errorf("postage-stamp threshold = %d; want 28", got)
	}
	// No patterns fall back to all five columns → 35.
	if got := HalftimeThreshold(nil); got != 35 {
		t.Errorf("no-pattern threshold = %d; want 35", got)
	}
}

// ── ClampAutoInterval ───────────────────────────────────────────────────────

func TestClampAutoInterval(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, DefaultAutoInterval},  // unset → default
		{-5, DefaultAutoInterval}, // negative → default
		{3, MinAutoInterval},      // below floor → floor
		{45, 45},                  // in range → unchanged
		{9999, MaxAutoInterval},   // above ceiling → ceiling
		{MinAutoInterval, MinAutoInterval},
		{MaxAutoInterval, MaxAutoInterval},
	}
	for _, c := range cases {
		if got := ClampAutoInterval(c.in); got != c.want {
			t.Errorf("ClampAutoInterval(%d) = %d; want %d", c.in, got, c.want)
		}
	}
}

// ── Auto state lifecycle ────────────────────────────────────────────────────

func TestAutoState_StartSeedsAndStamps(t *testing.T) {
	st := testStore(t)
	gs := NewService(st)
	patID, err := st.SavePattern("Top Row", testPattern5x5(), 1)
	if err != nil {
		t.Fatal(err)
	}

	// Starting with auto on seeds the state and stamps it onto the returned state.
	state, err := gs.Start([]int{int(patID)}, true, 20)
	if err != nil {
		t.Fatal(err)
	}
	if !state.AutoEnabled || state.AutoInterval != 20 {
		t.Fatalf("start auto stamp = (%v, %d); want (true, 20)", state.AutoEnabled, state.AutoInterval)
	}
	if enabled, interval := gs.AutoState(); !enabled || interval != 20 {
		t.Fatalf("AutoState = (%v, %d); want (true, 20)", enabled, interval)
	}

	// CurrentState also carries the auto snapshot.
	cur, _, err := gs.CurrentState()
	if err != nil {
		t.Fatal(err)
	}
	if !cur.AutoEnabled || cur.AutoInterval != 20 {
		t.Errorf("CurrentState auto = (%v, %d); want (true, 20)", cur.AutoEnabled, cur.AutoInterval)
	}

	// A fresh manual game resets auto off (clamped default interval).
	state2, err := gs.Start([]int{int(patID)}, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if state2.AutoEnabled {
		t.Error("manual start should leave auto off")
	}
	if state2.AutoInterval != DefaultAutoInterval {
		t.Errorf("reset interval = %d; want default %d", state2.AutoInterval, DefaultAutoInterval)
	}
}

func TestAutoState_ToggleAndInterval(t *testing.T) {
	gs := NewService(testStore(t))

	if got := gs.SetAutoInterval(15); got != 15 {
		t.Errorf("SetAutoInterval(15) = %d; want 15", got)
	}
	if got := gs.SetAutoInterval(1); got != MinAutoInterval {
		t.Errorf("SetAutoInterval(1) clamps to %d; got %d", MinAutoInterval, got)
	}
	if interval := gs.SetAutoEnabled(true); interval != MinAutoInterval {
		t.Errorf("SetAutoEnabled returned interval %d; want %d", interval, MinAutoInterval)
	}
	if enabled, _ := gs.AutoState(); !enabled {
		t.Error("expected auto enabled after SetAutoEnabled(true)")
	}
}

func TestAutoState_DisableReportsPriorState(t *testing.T) {
	gs := NewService(testStore(t))
	gs.SetAutoEnabled(true)
	if !gs.DisableAuto() {
		t.Error("DisableAuto should report wasEnabled=true")
	}
	if gs.DisableAuto() {
		t.Error("DisableAuto should report wasEnabled=false when already off")
	}
}

func TestAutoState_HalftimePauseResume(t *testing.T) {
	gs := NewService(testStore(t))

	// Nothing to pause when auto is off.
	if gs.PauseAutoForHalftime() {
		t.Error("PauseAutoForHalftime should be a no-op when auto is off")
	}

	// Pause a running loop, then resume it (the "no mini-game" path).
	gs.SetAutoEnabled(true)
	if !gs.PauseAutoForHalftime() {
		t.Fatal("expected PauseAutoForHalftime to pause a running loop")
	}
	if enabled, _ := gs.AutoState(); enabled {
		t.Error("auto should be paused after half-time")
	}
	if !gs.ResumeAutoAfterHalftime() {
		t.Fatal("expected ResumeAutoAfterHalftime to switch auto back on")
	}
	if enabled, _ := gs.AutoState(); !enabled {
		t.Error("auto should be running again after resume")
	}
	if gs.ResumeAutoAfterHalftime() {
		t.Error("second resume should be a no-op")
	}
}

func TestAutoState_HalftimeClearBlocksResume(t *testing.T) {
	gs := NewService(testStore(t))
	gs.SetAutoEnabled(true)
	gs.PauseAutoForHalftime()

	// Choosing a mini-game clears the resume flag: a later resume must not re-arm.
	gs.ClearHalftimeResume()
	if gs.ResumeAutoAfterHalftime() {
		t.Error("ClearHalftimeResume should prevent auto from resuming")
	}
	if enabled, _ := gs.AutoState(); enabled {
		t.Error("auto should stay off after ClearHalftimeResume")
	}
}

// ── Draw new-winner flag ────────────────────────────────────────────────────

func TestDraw_NewWinnerFlag(t *testing.T) {
	st := testStore(t)
	gs := NewService(st)
	patID, err := st.SavePattern("Top Row", testPattern5x5(), 1)
	if err != nil {
		t.Fatal(err)
	}
	// One card, so the first satisfying draw produces exactly one new winner.
	if err := st.SaveCard("TOPWIN", topRowWinningBoard()); err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Start([]int{int(patID)}, false, 0); err != nil {
		t.Fatal(err)
	}

	sawNewWinner := false
	for {
		result, newWinner, err := gs.Draw()
		if err != nil {
			t.Fatal(err)
		}
		if result == nil {
			break
		}
		if newWinner {
			if sawNewWinner {
				t.Error("new-winner flag should be true only on the winning draw, not again")
			}
			if len(result.Winners) == 0 {
				t.Error("new-winner flag set but no winners recorded")
			}
			sawNewWinner = true
		}
	}
	if !sawNewWinner {
		t.Error("expected the winning card to trigger the new-winner flag at least once")
	}
}

// topRowWinningBoard returns a board whose top row is B1,I16,N31,G46,O61 so the
// top-row pattern completes once those five numbers are drawn.
func topRowWinningBoard() [][]int {
	return [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
}
