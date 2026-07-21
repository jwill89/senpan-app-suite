package server_test

import (
	"net/http"
	"testing"
	"time"
)

// calledCount reads how many numbers have been drawn in the active game.
func calledCount(t *testing.T, env *testEnv) int {
	t.Helper()
	game, _ := decodeBody(t, env.get(t, "/api/game"))["game"].(map[string]any)
	if game == nil {
		return 0
	}
	called, _ := game["called_numbers"].([]any)
	return len(called)
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

// autoTestGame creates a top-row pattern (all five columns → half-time at 35) and
// returns its id, ready to start games from.
func autoTestPattern(t *testing.T, env *testEnv) int {
	t.Helper()
	resp := env.postJSON(t, "/api/patterns", map[string]any{
		"action": "create", "name": "Auto", "pattern_data": testPattern5x5(),
	})
	patID := decodeBody(t, resp)["pattern"].(map[string]any)["id"].(float64)
	return int(patID)
}

// autoTestBColumnPattern creates a pattern that marks only the B column, so the
// game has one active column (15 callable) and its half-time mark lands at 7 —
// low enough that a test can reach it quickly.
func autoTestBColumnPattern(t *testing.T, env *testEnv) int {
	t.Helper()
	grid := make([][]bool, 5)
	for r := range grid {
		grid[r] = make([]bool, 5)
		grid[r][0] = true
	}
	resp := env.postJSON(t, "/api/patterns", map[string]any{
		"action": "create", "name": "B Line", "pattern_data": grid,
	})
	return int(decodeBody(t, resp)["pattern"].(map[string]any)["id"].(float64))
}

// gameAuto reads the current game's auto flags from GET /api/game.
func gameAuto(t *testing.T, env *testEnv) (enabled bool, interval float64) {
	t.Helper()
	game, _ := decodeBody(t, env.get(t, "/api/game"))["game"].(map[string]any)
	if game == nil {
		t.Fatal("expected an active game")
	}
	enabled, _ = game["auto_enabled"].(bool)
	interval, _ = game["auto_interval"].(float64)
	return enabled, interval
}

// TestGame_StartWithAuto verifies a game started with auto reports it on the
// returned state, and that the interval is clamped into range.
func TestGame_StartWithAuto(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	patID := autoTestPattern(t, env)

	resp := env.postJSON(t, "/api/game/start", map[string]any{
		"pattern_ids": []int{patID}, "auto": true, "auto_interval": 3, // 3 → clamped up to 5
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	game, _ := decodeBody(t, resp)["game"].(map[string]any)
	if enabled, _ := game["auto_enabled"].(bool); !enabled {
		t.Error("expected auto_enabled=true on the started game")
	}
	if interval, _ := game["auto_interval"].(float64); interval != 5 {
		t.Errorf("auto_interval = %v; want 5 (clamped)", interval)
	}
}

// TestGame_PatchAuto verifies the live auto controls: enabling/disabling the loop
// and adjusting the interval, each reflected on GET /api/game.
func TestGame_PatchAuto(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	patID := autoTestPattern(t, env)
	env.postJSON(t, "/api/game/start", map[string]any{"pattern_ids": []int{patID}}).Body.Close()

	// Manual game starts with auto off.
	if enabled, _ := gameAuto(t, env); enabled {
		t.Fatal("expected auto off on a manual start")
	}

	// Turn auto on with an interval.
	env.patchJSON(t, "/api/game", map[string]any{"auto_enabled": true, "auto_interval": 15}).Body.Close()
	if enabled, interval := gameAuto(t, env); !enabled || interval != 15 {
		t.Errorf("after enable: (%v, %v); want (true, 15)", enabled, interval)
	}

	// Adjust the interval only (clamped).
	env.patchJSON(t, "/api/game", map[string]any{"auto_interval": 1}).Body.Close()
	if _, interval := gameAuto(t, env); interval != 5 {
		t.Errorf("after interval patch: %v; want 5 (clamped)", interval)
	}

	// Turn auto off.
	env.patchJSON(t, "/api/game", map[string]any{"auto_enabled": false}).Body.Close()
	if enabled, _ := gameAuto(t, env); enabled {
		t.Error("expected auto off after disable")
	}
}

// TestGame_HalftimePausesAndResumesAuto reaches the half-time mark via an auto
// draw (a manual draw would instead switch auto off), confirms auto was paused,
// then verifies that declining the mini-game resumes it. Numbers 1–6 are drawn
// manually with auto off; enabling auto then draws the 7th (the B-column half-time
// mark), which pauses the loop.
func TestGame_HalftimePausesAndResumesAuto(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	patID := autoTestBColumnPattern(t, env) // half-time at 7
	env.postJSON(t, "/api/game/start", map[string]any{"pattern_ids": []int{patID}}).Body.Close()

	go env.srv.RunAutoDrawScheduler(t.Context())

	// Manually draw to one short of the mark (auto off, so nothing to disable).
	for range 6 {
		env.postJSON(t, "/api/game/draw", map[string]any{}).Body.Close()
	}
	if n := calledCount(t, env); n != 6 {
		t.Fatalf("expected 6 numbers drawn before enabling auto; got %d", n)
	}

	// Enable auto → the immediate auto draw crosses the half-time mark and pauses.
	env.patchJSON(t, "/api/game", map[string]any{"auto_enabled": true, "auto_interval": 600}).Body.Close()
	waitForCalled(t, env, 7, 2*time.Second)
	if enabled, _ := gameAuto(t, env); enabled {
		t.Error("auto should be paused at the half-time mark")
	}

	// Decline the mini-game → auto resumes.
	env.postJSON(t, "/api/game/halftime", map[string]any{"minigame": false}).Body.Close()
	if enabled, _ := gameAuto(t, env); !enabled {
		t.Error("declining the mini-game should resume auto")
	}
}

// TestGame_ManualDrawDisablesAuto verifies that manually drawing during an
// auto-run game takes over — switching auto off.
func TestGame_ManualDrawDisablesAuto(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	patID := autoTestPattern(t, env)
	env.postJSON(t, "/api/game/start", map[string]any{
		"pattern_ids": []int{patID}, "auto": true, "auto_interval": 600,
	}).Body.Close()
	if enabled, _ := gameAuto(t, env); !enabled {
		t.Fatal("expected auto on after start-with-auto")
	}

	// A single manual draw takes over → auto switches off.
	env.postJSON(t, "/api/game/draw", map[string]any{}).Body.Close()
	if enabled, _ := gameAuto(t, env); enabled {
		t.Error("a manual draw should disable auto-draw")
	}
}

// TestGame_DisablingAutoCancelsPendingDraw confirms that turning auto off cancels
// the scheduler's pending draw — no stray number appears after the interval that
// was counting down.
func TestGame_DisablingAutoCancelsPendingDraw(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	patID := autoTestPattern(t, env)
	env.postJSON(t, "/api/game/start", map[string]any{"pattern_ids": []int{patID}}).Body.Close()

	go env.srv.RunAutoDrawScheduler(t.Context())

	// Enable auto with the shortest interval, so a second draw is imminent.
	env.patchJSON(t, "/api/game", map[string]any{"auto_enabled": true, "auto_interval": 5}).Body.Close()
	waitForCalled(t, env, 1, 2*time.Second) // the immediate draw
	env.patchJSON(t, "/api/game", map[string]any{"auto_enabled": false}).Body.Close()
	n := calledCount(t, env)

	// Wait past the 5s interval — the cancelled timer must not draw again.
	time.Sleep(6 * time.Second)
	if got := calledCount(t, env); got != n {
		t.Errorf("auto drew after being disabled (%d → %d); the pending timer was not cancelled", n, got)
	}
}

// TestGame_AutoDrawsImmediatelyOnEnable verifies that toggling auto on mid-game
// draws the first number right away (not after a full interval), then holds off —
// the delay/interval never postpone that first draw.
func TestGame_AutoDrawsImmediatelyOnEnable(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	patID := autoTestPattern(t, env)
	// Manual start (auto off), no cards so no winner can interfere.
	env.postJSON(t, "/api/game/start", map[string]any{"pattern_ids": []int{patID}}).Body.Close()

	go env.srv.RunAutoDrawScheduler(t.Context())

	if n := calledCount(t, env); n != 0 {
		t.Fatalf("expected 0 numbers drawn before auto is on; got %d", n)
	}

	// Turn auto on with the maximum interval — only the immediate draw should land.
	env.patchJSON(t, "/api/game", map[string]any{"auto_enabled": true, "auto_interval": 600}).Body.Close()

	// First number arrives ~immediately.
	waitForCalled(t, env, 1, 2*time.Second)

	// …and it does not keep drawing (next draw is a full 600s away).
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

	env.postJSON(t, "/api/game/start", map[string]any{
		"pattern_ids": []int{patID}, "auto": true, "auto_interval": 600,
	}).Body.Close()

	waitForCalled(t, env, 1, 2*time.Second)
}

// TestGame_HalftimeMinigameKeepsAutoOff confirms that choosing to run a mini-game
// leaves auto paused (the admin re-enables it manually when ready). Same setup as
// TestGame_HalftimePausesAndResumesAuto: the immediate auto draw crosses the mark.
func TestGame_HalftimeMinigameKeepsAutoOff(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	patID := autoTestBColumnPattern(t, env) // half-time at 7
	env.postJSON(t, "/api/game/start", map[string]any{"pattern_ids": []int{patID}}).Body.Close()

	go env.srv.RunAutoDrawScheduler(t.Context())

	for range 6 {
		env.postJSON(t, "/api/game/draw", map[string]any{}).Body.Close()
	}
	env.patchJSON(t, "/api/game", map[string]any{"auto_enabled": true, "auto_interval": 600}).Body.Close()
	waitForCalled(t, env, 7, 2*time.Second)
	if enabled, _ := gameAuto(t, env); enabled {
		t.Fatal("auto should be paused at the half-time mark")
	}

	// Choose the mini-game → auto stays off.
	env.postJSON(t, "/api/game/halftime", map[string]any{"minigame": true}).Body.Close()
	if enabled, _ := gameAuto(t, env); enabled {
		t.Error("running a mini-game should leave auto off until re-enabled")
	}
}
