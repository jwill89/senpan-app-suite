package server_test

import (
	"net/http"
	"testing"
	"time"
)

// calledCount reads how many numbers have been drawn in the active game.
func calledCount(t *testing.T, env *testEnv) int {
	t.Helper()
	if g := readGame(t, env.client, env).Game; g != nil {
		return len(g.CalledNumbers)
	}
	return 0
}

// waitForCalled polls until at least `want` numbers have been drawn, or fails.
func waitForCalled(t *testing.T, env *testEnv, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if calledCount(t, env) >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d called number(s); have %d", want, calledCount(t, env))
}

// waitForHalftimePause polls until the auto loop has paused itself for half-time,
// or fails. The loop pauses just after the crossing draw commits, so a read can
// see that draw a moment before the pause.
func waitForHalftimePause(t *testing.T, env *testEnv, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if enabled, _ := gameAuto(t, env); !enabled {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("auto should be paused at the half-time mark")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// autoTestPattern creates a top-row pattern (all five columns -> half-time at 35)
// and returns its id, ready to start games from.
func autoTestPattern(t *testing.T, env *testEnv) int64 {
	t.Helper()
	return createPattern(t, env, "Auto", testPattern5x5())
}

// gameAuto reads the current game's auto flags.
func gameAuto(t *testing.T, env *testEnv) (enabled bool, interval int) {
	t.Helper()
	g := readGame(t, env.client, env).Game
	if g == nil {
		t.Fatal("expected an active game")
	}
	return g.AutoEnabled, g.AutoInterval
}

// startManual starts a game with the automatic draws off.
func startManual(t *testing.T, env *testEnv, pattern int64) gameView {
	t.Helper()
	return startGame(t, env, pattern)
}

// TestGame_StartWithAuto verifies a game started with auto reports it on the
// returned state, and that the interval is clamped into range.
func TestGame_StartWithAuto(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	patID := autoTestPattern(t, env)

	var s gameState
	decodeInto(t, env.postJSON(t, "/api/v1/game/start", map[string]any{
		"patternIds": []int64{patID}, "auto": true, "autoInterval": 3, // 3 -> clamped up to 5
	}), http.StatusOK, &s)
	if !s.Game.AutoEnabled {
		t.Error("expected autoEnabled on the started game")
	}
	if s.Game.AutoInterval != 5 {
		t.Errorf("autoInterval = %v; want 5 (clamped)", s.Game.AutoInterval)
	}
}

// TestGame_PatchAuto verifies the live auto controls: enabling/disabling the loop
// and adjusting the interval, each reflected on the game.
func TestGame_PatchAuto(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	startManual(t, env, autoTestPattern(t, env))

	if enabled, _ := gameAuto(t, env); enabled {
		t.Fatal("expected auto off on a manual start")
	}

	env.patchJSON(t, "/api/v1/game", map[string]any{"autoEnabled": true, "autoInterval": 15}).Body.Close()
	if enabled, interval := gameAuto(t, env); !enabled || interval != 15 {
		t.Errorf("after enable: (%v, %v); want (true, 15)", enabled, interval)
	}

	env.patchJSON(t, "/api/v1/game", map[string]any{"autoInterval": 1}).Body.Close()
	if _, interval := gameAuto(t, env); interval != 5 {
		t.Errorf("after interval patch: %v; want 5 (clamped)", interval)
	}

	env.patchJSON(t, "/api/v1/game", map[string]any{"autoEnabled": false}).Body.Close()
	if enabled, _ := gameAuto(t, env); enabled {
		t.Error("expected auto off after disable")
	}
}

// TestGame_HalftimePausesAndResumesAuto reaches the half-time mark via an auto
// draw (a manual draw would instead switch auto off), confirms auto was paused,
// then verifies that declining the mini-game resumes it. Numbers 1-6 are drawn
// manually with auto off; enabling auto then draws the 7th (the B-column half-time
// mark), which pauses the loop.
func TestGame_HalftimePausesAndResumesAuto(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	g := startManual(t, env, bColumnPattern(t, env)) // half-time at 7

	go env.srv.RunAutoDrawScheduler(t.Context())

	for range 6 {
		drawNumber(t, env, 0)
	}
	if n := calledCount(t, env); n != 6 {
		t.Fatalf("expected 6 numbers drawn before enabling auto; got %d", n)
	}

	// Enable auto -> the immediate auto draw crosses the half-time mark and pauses.
	env.patchJSON(t, "/api/v1/game", map[string]any{"autoEnabled": true, "autoInterval": 600}).Body.Close()
	waitForCalled(t, env, 7, 2*time.Second)
	waitForHalftimePause(t, env, 2*time.Second)
	if h := readGame(t, env.client, env).Game.Halftime; h == nil || !h.AutoPaused {
		t.Errorf("the prompt waiting: %+v; want one saying the draws were paused", h)
	}

	// Decline the mini-game -> auto resumes.
	decodeInto(t, env.postJSON(t, "/api/v1/game/halftime", map[string]any{"gameId": g.ID, "minigame": false}), http.StatusOK, nil)
	if enabled, _ := gameAuto(t, env); !enabled {
		t.Error("declining the mini-game should resume auto")
	}
}

// TestGame_ManualDrawDisablesAuto verifies that manually drawing during an
// auto-run game takes over - switching auto off.
func TestGame_ManualDrawDisablesAuto(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	decodeInto(t, env.postJSON(t, "/api/v1/game/start", map[string]any{
		"patternIds": []int64{autoTestPattern(t, env)}, "auto": true, "autoInterval": 600,
	}), http.StatusOK, nil)
	if enabled, _ := gameAuto(t, env); !enabled {
		t.Fatal("expected auto on after start-with-auto")
	}

	drawNumber(t, env, 0)
	if enabled, _ := gameAuto(t, env); enabled {
		t.Error("a manual draw should disable auto-draw")
	}
}

// TestGame_DisablingAutoCancelsPendingDraw confirms that turning auto off cancels
// the scheduler's pending draw - no stray number appears after the interval that
// was counting down.
func TestGame_DisablingAutoCancelsPendingDraw(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	startManual(t, env, autoTestPattern(t, env))

	go env.srv.RunAutoDrawScheduler(t.Context())

	env.patchJSON(t, "/api/v1/game", map[string]any{"autoEnabled": true, "autoInterval": 5}).Body.Close()
	waitForCalled(t, env, 1, 2*time.Second) // the immediate draw
	env.patchJSON(t, "/api/v1/game", map[string]any{"autoEnabled": false}).Body.Close()
	n := calledCount(t, env)

	// Wait past the 5s interval - the cancelled timer must not draw again.
	time.Sleep(6 * time.Second)
	if got := calledCount(t, env); got != n {
		t.Errorf("auto drew after being disabled (%d -> %d); the pending timer was not cancelled", n, got)
	}
}

// TestGame_AutoDrawsImmediatelyOnEnable verifies that toggling auto on mid-game
// draws the first number right away (not after a full interval), then holds off -
// the delay/interval never postpone that first draw.
func TestGame_AutoDrawsImmediatelyOnEnable(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	startManual(t, env, autoTestPattern(t, env))

	go env.srv.RunAutoDrawScheduler(t.Context())

	if n := calledCount(t, env); n != 0 {
		t.Fatalf("expected 0 numbers drawn before auto is on; got %d", n)
	}

	env.patchJSON(t, "/api/v1/game", map[string]any{"autoEnabled": true, "autoInterval": 600}).Body.Close()
	waitForCalled(t, env, 1, 2*time.Second)

	time.Sleep(300 * time.Millisecond)
	if n := calledCount(t, env); n != 1 {
		t.Errorf("expected exactly 1 draw (the immediate one); got %d", n)
	}
}

// TestGame_AutoDrawsImmediatelyOnStart verifies that starting a game with auto on
// also draws the first number immediately.
func TestGame_AutoDrawsImmediatelyOnStart(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	patID := autoTestPattern(t, env)

	go env.srv.RunAutoDrawScheduler(t.Context())

	decodeInto(t, env.postJSON(t, "/api/v1/game/start", map[string]any{
		"patternIds": []int64{patID}, "auto": true, "autoInterval": 600,
	}), http.StatusOK, nil)

	waitForCalled(t, env, 1, 2*time.Second)
}

// TestGame_HalftimeMinigameKeepsAutoOff confirms that choosing to run a mini-game
// leaves auto paused (the admin re-enables it manually when ready). Same setup as
// TestGame_HalftimePausesAndResumesAuto: the immediate auto draw crosses the mark.
func TestGame_HalftimeMinigameKeepsAutoOff(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	g := startManual(t, env, bColumnPattern(t, env)) // half-time at 7

	go env.srv.RunAutoDrawScheduler(t.Context())

	for range 6 {
		drawNumber(t, env, 0)
	}
	env.patchJSON(t, "/api/v1/game", map[string]any{"autoEnabled": true, "autoInterval": 600}).Body.Close()
	waitForCalled(t, env, 7, 2*time.Second)
	waitForHalftimePause(t, env, 2*time.Second)

	decodeInto(t, env.postJSON(t, "/api/v1/game/halftime", map[string]any{"gameId": g.ID, "minigame": true}), http.StatusOK, nil)
	if enabled, _ := gameAuto(t, env); enabled {
		t.Error("running a mini-game should leave auto off until re-enabled")
	}
}
