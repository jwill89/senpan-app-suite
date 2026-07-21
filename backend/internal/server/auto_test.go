package server_test

import (
	"net/http"
	"testing"
)

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

// TestGame_HalftimePausesAndResumesAuto walks a game to the half-time mark (with
// no cards, so no winner interferes), confirms auto was paused, then verifies that
// declining the mini-game resumes it.
func TestGame_HalftimePausesAndResumesAuto(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	patID := autoTestPattern(t, env)
	env.postJSON(t, "/api/game/start", map[string]any{
		"pattern_ids": []int{patID}, "auto": true, "auto_interval": 10,
	}).Body.Close()

	// Draw until the half-time crossing pauses auto (top row → threshold 35).
	paused := false
	for range 75 {
		env.postJSON(t, "/api/game/draw", map[string]any{}).Body.Close()
		if enabled, _ := gameAuto(t, env); !enabled {
			paused = true
			break
		}
	}
	if !paused {
		t.Fatal("auto was never paused at the half-time mark")
	}

	// Decline the mini-game → auto resumes.
	env.postJSON(t, "/api/game/halftime", map[string]any{"minigame": false}).Body.Close()
	if enabled, _ := gameAuto(t, env); !enabled {
		t.Error("declining the mini-game should resume auto")
	}
}

// TestGame_HalftimeMinigameKeepsAutoOff confirms that choosing to run a mini-game
// leaves auto paused (the admin re-enables it manually when ready).
func TestGame_HalftimeMinigameKeepsAutoOff(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	patID := autoTestPattern(t, env)
	env.postJSON(t, "/api/game/start", map[string]any{
		"pattern_ids": []int{patID}, "auto": true, "auto_interval": 10,
	}).Body.Close()

	paused := false
	for range 75 {
		env.postJSON(t, "/api/game/draw", map[string]any{}).Body.Close()
		if enabled, _ := gameAuto(t, env); !enabled {
			paused = true
			break
		}
	}
	if !paused {
		t.Fatal("auto was never paused at the half-time mark")
	}

	// Choose the mini-game → auto stays off.
	env.postJSON(t, "/api/game/halftime", map[string]any{"minigame": true}).Body.Close()
	if enabled, _ := gameAuto(t, env); enabled {
		t.Error("running a mini-game should leave auto off until re-enabled")
	}
}
