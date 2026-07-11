package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// readWsType reads WebSocket frames until it sees one whose "type" equals want,
// decoding it into a generic map. Other message types (and non-JSON keepalives)
// are skipped so the assertion is robust to unrelated traffic. Fails on timeout.
func readWsType(t *testing.T, conn *websocket.Conn, want string) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("ws read (waiting for %q): %v", want, err)
		}
		var msg map[string]any
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		if msg["type"] == want {
			return msg
		}
	}
}

// topRowPattern is a top-row win pattern grid.
func topRowPattern() [][]bool {
	grid := make([][]bool, 5)
	for i := range grid {
		grid[i] = make([]bool, 5)
	}
	for c := 0; c < 5; c++ {
		grid[0][c] = true
	}
	return grid
}

// startYoeverGame seeds a pattern and starts a game through the API (so the
// server's game service resets the reaction state to enabled), returning a
// freshly-created named card's id to trigger with.
func (e *testEnv) startYoeverGame(t *testing.T, playerName string) string {
	t.Helper()
	patID, err := e.store.SavePattern("Top Row", topRowPattern(), 1)
	if err != nil {
		t.Fatal(err)
	}
	resp := e.postJSON(t, "/api/game/start", map[string]any{"pattern_ids": []int{int(patID)}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start game status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = e.postJSON(t, "/api/cards", map[string]any{"player_name": playerName})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create card status = %d; want 201", resp.StatusCode)
	}
	card, _ := decodeBody(t, resp)["card"].(map[string]any)
	id, _ := card["id"].(string)
	if id == "" {
		t.Fatal("expected a card id")
	}
	return id
}

// TestYoever_TriggerBroadcastsToClients verifies that a player's public trigger
// increments the per-game count, is throttled by the cooldown, and pushes a
// "yoever" frame (with the triggering player's name) to every connected client.
func TestYoever_TriggerBroadcastsToClients(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	cardID := env.startYoeverGame(t, "Tifa")

	conn := env.dialAdminWS(t)
	defer conn.Close(websocket.StatusNormalClosure, "")

	resp := env.postJSON(t, "/api/game/yoever", map[string]any{"card_id": cardID})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("yoever status = %d; want 200", resp.StatusCode)
	}
	body := decodeBody(t, resp)
	if body["count"].(float64) != 1 {
		t.Errorf("count = %v; want 1", body["count"])
	}

	msg := readWsType(t, conn, "yoever")
	if msg["player_name"] != "Tifa" {
		t.Errorf("player_name = %v; want Tifa", msg["player_name"])
	}
	if msg["count"].(float64) != 1 {
		t.Errorf("broadcast count = %v; want 1", msg["count"])
	}

	// A second trigger from the same card inside the cooldown is rejected (429).
	resp = env.postJSON(t, "/api/game/yoever", map[string]any{"card_id": cardID})
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second trigger status = %d; want 429", resp.StatusCode)
	}
	if got := decodeBody(t, resp)["retry_after"].(float64); got <= 0 {
		t.Errorf("retry_after = %v; want > 0", got)
	}
}

// TestYoever_DisabledTogglePreventsTrigger verifies the admin on/off switch:
// PATCH /api/game {yoever_enabled:false} broadcasts yoever_config and makes the
// public trigger return 403.
func TestYoever_DisabledTogglePreventsTrigger(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	cardID := env.startYoeverGame(t, "Aerith")

	conn := env.dialAdminWS(t)
	defer conn.Close(websocket.StatusNormalClosure, "")

	resp := env.patchJSON(t, "/api/game", map[string]any{"yoever_enabled": false})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("toggle status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	msg := readWsType(t, conn, "yoever_config")
	if msg["enabled"] != false {
		t.Errorf("yoever_config enabled = %v; want false", msg["enabled"])
	}

	resp = env.postJSON(t, "/api/game/yoever", map[string]any{"card_id": cardID})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("disabled trigger status = %d; want 403", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestYoever_NoActiveGameConflict verifies the trigger returns 409 when no game
// is running, and 404 for an unknown board id.
func TestYoever_NoActiveGameConflict(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// A real card, but no active game.
	resp := env.postJSON(t, "/api/cards", map[string]any{"player_name": "Yuffie"})
	cardID, _ := decodeBody(t, resp)["card"].(map[string]any)["id"].(string)

	resp = env.postJSON(t, "/api/game/yoever", map[string]any{"card_id": cardID})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("no-game trigger status = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()

	resp = env.postJSON(t, "/api/game/yoever", map[string]any{"card_id": "NOPE99"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown-card status = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()
}
