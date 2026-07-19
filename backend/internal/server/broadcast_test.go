package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"app-suite/internal/model"

	"github.com/coder/websocket"
)

// ── WebSocket test helpers ──────────────────────────────────────────────────
//
// These exercise the live-admin invalidation path end-to-end: an authenticated
// admin opens the privileged WebSocket channel, a *public* mutation happens, and
// the admin should receive a thin "resource_changed" signal (see
// server.broadcastResourceChanged). The public draw/enter paths are excluded from
// the adminMutationResource middleware, so the handlers broadcast explicitly —
// these tests guard that wiring.

// dialAdminWS opens an authenticated admin WebSocket (no card id) to the test
// server and returns the connection. The caller must already be logged in as an
// admin — the session cookie rides on e.client's jar, and websocket.Dial reuses
// that client (TLS pool + cookies). It then waits until the hub has registered
// the connection so a subsequent broadcast can't race ahead of registration.
func (e *testEnv) dialAdminWS(t *testing.T) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// websocket.Dial interprets the https URL as wss.
	conn, _, err := websocket.Dial(ctx, e.url("/api/ws"), &websocket.DialOptions{HTTPClient: e.client})
	if err != nil {
		t.Fatalf("dial admin ws: %v", err)
	}

	// ServeWS registers the client just after the handshake; Dial can return
	// before that completes, so wait for the hub to see it before proceeding.
	deadline := time.Now().Add(2 * time.Second)
	for e.hub.ClientCount() == 0 {
		if time.Now().After(deadline) {
			conn.Close(websocket.StatusInternalError, "")
			t.Fatal("admin ws never registered with hub")
		}
		time.Sleep(2 * time.Millisecond)
	}
	return conn
}

// expectResourceChanged reads WS frames until it sees a "resource_changed" for
// want (failing on a mismatched resource), or fails on timeout. Non-JSON/other
// message types are skipped so the assertion is robust to unrelated traffic.
func expectResourceChanged(t *testing.T, conn *websocket.Conn, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("ws read (waiting for resource_changed %q): %v", want, err)
		}
		var msg struct {
			Type     string `json:"type"`
			Resource string `json:"resource"`
		}
		if json.Unmarshal(data, &msg) != nil || msg.Type != "resource_changed" {
			continue
		}
		if msg.Resource != want {
			t.Fatalf("resource_changed resource = %q; want %q", msg.Resource, want)
		}
		return
	}
}

// expectCardsUpdate reads WS frames until a cards_update carrying card id, and
// returns that entry so tests can assert on its status fields. Skips unrelated
// message types.
func expectCardsUpdate(t *testing.T, conn *websocket.Conn, id string) model.CardListEntry {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("ws read (waiting for cards_update %q): %v", id, err)
		}
		var msg struct {
			Type  string                `json:"type"`
			Cards []model.CardListEntry `json:"cards"`
		}
		if json.Unmarshal(data, &msg) != nil || msg.Type != "cards_update" {
			continue
		}
		for _, c := range msg.Cards {
			if c.ID == id {
				return c
			}
		}
		t.Fatalf("cards_update did not contain card %q", id)
	}
}

// ── Card status broadcasts (regression: the cards_update payload must carry the
// full card shape, not a subset) ───────────────────────────────────────────────

// TestCardProtectBroadcastsProtected guards against the cards_update broadcast
// dropping status fields. Protecting a card must push a cards_update whose entry
// has protected=true — a dropped field made the Protected lock lag until a manual
// refetch, and only in one direction (the omitted field always read false, so
// unprotect "worked" but protect didn't).
func TestCardProtectBroadcastsProtected(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	if err := env.store.SaveCard("PROT01", srvValidBoard()); err != nil {
		t.Fatal(err)
	}

	conn := env.dialAdminWS(t)
	defer conn.Close(websocket.StatusNormalClosure, "")

	resp := env.postJSON(t, "/api/cards/PROT01/protect", map[string]any{"protected": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("protect status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	if card := expectCardsUpdate(t, conn, "PROT01"); !card.Protected {
		t.Error("cards_update dropped protected=true (the lock icon would lag until a refetch)")
	}
}

// TestCardApproveBroadcastsStatus verifies approving a pending custom card pushes a
// cards_update carrying custom_status="approved" and protected=true, so the star +
// lock update live rather than only after a manual refetch.
func TestCardApproveBroadcastsStatus(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	if err := env.store.CreateCustomCard("CUST01", srvValidBoard(), "Aria", "Gilgamesh"); err != nil {
		t.Fatal(err)
	}

	conn := env.dialAdminWS(t)
	defer conn.Close(websocket.StatusNormalClosure, "")

	resp := env.postJSON(t, "/api/cards/CUST01/approve", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	card := expectCardsUpdate(t, conn, "CUST01")
	if card.CustomStatus != "approved" {
		t.Errorf("cards_update custom_status = %q; want approved", card.CustomStatus)
	}
	if !card.Protected {
		t.Error("cards_update dropped protected=true for the approved custom card")
	}
}

// ── Garapon draw broadcast ──────────────────────────────────────────────────

// TestGarapon_DrawBroadcastsResourceChanged verifies that a public draw (which
// goes through the token path, excluded from the adminMutationResource
// middleware) still pushes a "garapons" invalidation so an admin viewing the
// garapon detail refetches the draw log live.
func TestGarapon_DrawBroadcastsResourceChanged(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Seed a garapon + a drawing link directly through the store.
	gid, err := env.store.CreateGarapon(&model.Garapon{
		Title:  "Festival Drum",
		Prizes: []model.GaraponPrize{{Name: "Grand", BallColor: "#fff", Rate: 1, IsGrand: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	player, err := env.store.CreateGaraponPlayer(gid, "Hero", 3)
	if err != nil {
		t.Fatal(err)
	}

	conn := env.dialAdminWS(t)
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Public draw — note the singular "garapon" token path.
	resp := env.postJSON(t, "/api/garapon/"+player.Token+"/draw", map[string]any{})
	if resp.StatusCode != 200 {
		t.Fatalf("draw status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	expectResourceChanged(t, conn, "garapons")
}

// ── Raffle enter broadcast ──────────────────────────────────────────────────

// TestRaffle_EnterBroadcastsResourceChanged verifies that a public raffle sign-up
// (the ".../enter" path, excluded from the adminMutationResource middleware)
// pushes a "raffles" invalidation so an admin viewing the raffle detail refetches
// the entry list live.
func TestRaffle_EnterBroadcastsResourceChanged(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	rid, err := env.store.CreateRaffle(&model.Raffle{Title: "Prize Raffle", MaxEntries: 10})
	if err != nil {
		t.Fatal(err)
	}

	conn := env.dialAdminWS(t)
	defer conn.Close(websocket.StatusNormalClosure, "")

	resp := env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", rid), map[string]any{
		"character_name": "Cloud", "world": "Gaia", "num_entries": 1,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("enter status = %d; want 201", resp.StatusCode)
	}
	resp.Body.Close()

	expectResourceChanged(t, conn, "raffles")
}
