package server_test

import (
	"net/http"
	"strconv"
	"testing"
	"time"
)

// topRowPattern is a top-row win pattern grid.
func topRowPattern() [][]bool {
	grid := make([][]bool, 5)
	for i := range grid {
		grid[i] = make([]bool, 5)
	}
	for c := range 5 {
		grid[0][c] = true
	}
	return grid
}

// startYoeverGame seeds a pattern and starts a game through the API (so the
// server's game service resets the reaction state to enabled), returning a
// freshly-created named card's id to trigger with.
func (e *testEnv) startYoeverGame(t *testing.T, playerName string) string {
	t.Helper()
	patID, err := e.store.SavePattern(t.Context(), "Top Row", topRowPattern(), 1)
	if err != nil {
		t.Fatal(err)
	}
	startGame(t, e, patID)
	return makeCard(t, e, playerName).ID
}

// TestYoever_TriggerReachesTheStreams verifies that a player's public trigger
// increments the per-game count, is throttled by the cooldown, and sends a "yoever"
// event (with the triggering player's name) to the staff's and the boards' streams.
func TestYoever_TriggerReachesTheStreams(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	cardID := env.startYoeverGame(t, "Tifa")

	staff := staffStream(t, env, env.client)
	board := boardStream(t, env, cardID)

	player := newClient(t, env)
	var got struct {
		Count           int
		CooldownSeconds int
	}
	decodeInto(t, postAs(t, player, env, "/api/v1/game/yoever", map[string]any{"cardId": cardID}), http.StatusOK, &got)
	if got.Count != 1 || got.CooldownSeconds != 180 {
		t.Errorf("answer = %+v; want count 1, cooldown 180", got)
	}

	for name, s := range map[string]*stream{"staff": staff, "board": board} {
		e := s.expect("yoever", 3*time.Second)
		if e.str("playerName") != "Tifa" || e.Payload["count"] != float64(1) || e.game() == 0 {
			t.Errorf("%s's yoever: %+v; want Tifa, count 1, the game", name, e)
		}
	}

	// A second trigger from the same card inside the cooldown is refused (429), saying
	// when to try again.
	resp := postAs(t, player, env, "/api/v1/game/yoever", map[string]any{"cardId": cardID})
	if _, status, _ := problem(t, resp); status != http.StatusTooManyRequests {
		t.Fatalf("second trigger status = %d; want 429", status)
	}
	if after, err := strconv.Atoi(resp.Header.Get("Retry-After")); err != nil || after <= 0 {
		t.Errorf("Retry-After = %q; want a number of seconds", resp.Header.Get("Retry-After"))
	}
}

// TestYoever_DisabledTogglePreventsTrigger verifies the admin on/off switch:
// PATCH /api/v1/game {yoeverEnabled:false} sends yoever_switched and makes the
// public trigger return 403.
func TestYoever_DisabledTogglePreventsTrigger(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	cardID := env.startYoeverGame(t, "Aerith")

	live := staffStream(t, env, env.client)

	var s gameState
	decodeInto(t, env.patchJSON(t, "/api/v1/game", map[string]any{"yoeverEnabled": false}), http.StatusOK, &s)
	if s.Game.YoeverEnabled {
		t.Error("the answer still has the reaction on")
	}

	if e := live.expect("yoever_switched", 3*time.Second); e.Payload["enabled"] != false {
		t.Errorf("yoever_switched: %+v; want enabled false", e)
	}

	if _, status, _ := problem(t, env.postJSON(t, "/api/v1/game/yoever", map[string]any{"cardId": cardID})); status != http.StatusForbidden {
		t.Fatalf("disabled trigger status = %d; want 403", status)
	}
}

// TestYoever_NoActiveGameConflict verifies the trigger returns 409 when no game
// is running, 404 for an unknown board id, and 422 for one that isn't an id.
func TestYoever_NoActiveGameConflict(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	cardID := makeCard(t, env, "Yuffie").ID

	if _, status, _ := problem(t, env.postJSON(t, "/api/v1/game/yoever", map[string]any{"cardId": cardID})); status != http.StatusConflict {
		t.Fatalf("no-game trigger status = %d; want 409", status)
	}
	if _, status, _ := problem(t, env.postJSON(t, "/api/v1/game/yoever", map[string]any{"cardId": "NOPE99"})); status != http.StatusNotFound {
		t.Fatalf("unknown-card status = %d; want 404", status)
	}
	if at := invalid(t, env.postJSON(t, "/api/v1/game/yoever", map[string]any{"cardId": ""})); len(at) == 0 || at[0] != "body.cardId" {
		t.Errorf("an empty id: errors at %v", at)
	}
}
