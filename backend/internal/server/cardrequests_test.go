package server_test

import (
	"fmt"
	"net/http"
	"testing"
)

// srvValidBoard returns a fresh structurally valid 5×5 bingo board (centre = FREE).
func srvValidBoard() [][]int {
	return [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
}

func TestCardRequestPublicFlow(t *testing.T) {
	env := newTestEnv(t)

	// A valid request is accepted and stored as pending.
	resp := env.postJSON(t, "/api/cards/request", map[string]any{
		"character_name": "Aria Nightsong",
		"world":          "Gilgamesh",
		"card_id":        "tst001", // lower-case → normalised to TST001
		"board_data":     srvValidBoard(),
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("valid request = %d; want 201", resp.StatusCode)
	}
	got := decodeBody(t, resp)
	if got["id"] != "TST001" || got["status"] != "pending" {
		t.Fatalf("response = %v; want id=TST001 status=pending", got)
	}

	// A pending card is stored but NOT playable on the public board.
	resp = env.get(t, "/api/board?id=TST001")
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("pending board join = %d; want 403", resp.StatusCode)
	}
	resp.Body.Close()

	// A taken ID is rejected (different board so only the ID collides).
	dupIDBoard := srvValidBoard()
	dupIDBoard[0][0] = 6
	resp = env.postJSON(t, "/api/cards/request", map[string]any{
		"character_name": "X", "world": "Gilgamesh", "card_id": "TST001", "board_data": dupIDBoard,
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate id = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()

	// A board identical to an existing card is rejected (different ID).
	resp = env.postJSON(t, "/api/cards/request", map[string]any{
		"character_name": "X", "world": "Gilgamesh", "card_id": "NEW999", "board_data": srvValidBoard(),
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate board = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()

	// Structurally invalid board (99 out of the B range) is rejected.
	badBoard := srvValidBoard()
	badBoard[0][0] = 99
	resp = env.postJSON(t, "/api/cards/request", map[string]any{
		"character_name": "X", "world": "Gilgamesh", "card_id": "BAD001", "board_data": badBoard,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid board = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Bad card ID (too long) is rejected.
	resp = env.postJSON(t, "/api/cards/request", map[string]any{
		"character_name": "X", "world": "Gilgamesh", "card_id": "TOOLONG9", "board_data": srvValidBoard(),
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad id = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Missing character/world is rejected.
	resp = env.postJSON(t, "/api/cards/request", map[string]any{
		"character_name": "", "world": "", "card_id": "OKID12", "board_data": srvValidBoard(),
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing fields = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCardApproveAndProtect(t *testing.T) {
	env := newTestEnv(t)

	// Submit a pending custom card (public).
	env.postJSON(t, "/api/cards/request", map[string]any{
		"character_name": "Aria", "world": "Gilgamesh", "card_id": "CUST01", "board_data": srvValidBoard(),
	}).Body.Close()

	// Approve requires the bingo-cards permission.
	resp := env.postJSON(t, "/api/cards/CUST01/approve", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("approve unauth = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()

	env.loginAdmin(t)

	// Approve the pending card → approved + protected.
	resp = env.postJSON(t, "/api/cards/CUST01/approve", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
	card, err := env.store.GetCard("CUST01")
	if err != nil || card == nil {
		t.Fatal("card missing after approve")
	}
	if card.CustomStatus != "approved" || !card.Protected {
		t.Errorf("approved card = status %q protected %v; want approved/true", card.CustomStatus, card.Protected)
	}

	// An approved card is now playable.
	resp = env.get(t, "/api/board?id=CUST01")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("approved board join = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Re-approving a non-pending card is a no-op (404).
	resp = env.postJSON(t, "/api/cards/CUST01/approve", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("re-approve = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()

	// Protect toggles a normal card.
	if err := env.store.SaveCard("NORM01", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	resp = env.postJSON(t, "/api/cards/NORM01/protect", map[string]any{"protected": true})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("protect = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
	if c, _ := env.store.GetCard("NORM01"); c == nil || !c.Protected {
		t.Error("card should be Protected after protect")
	}

	// Protecting a missing card → 404.
	resp = env.postJSON(t, "/api/cards/NOPE99/protect", map[string]any{"protected": true})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("protect missing = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDeleteAllSparesProtected(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	for _, id := range []string{"AAA111", "BBB222"} {
		if err := env.store.SaveCard(id, srvValidBoard()); err != nil {
			t.Fatal(err)
		}
	}
	if err := env.store.SaveCard("KEEP01", srvValidBoard()); err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.SetCardProtected("KEEP01", true); err != nil {
		t.Fatal(err)
	}

	resp := env.del(t, "/api/cards/all")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete all = %d; want 200", resp.StatusCode)
	}
	got := decodeBody(t, resp)
	if got["deleted"].(float64) != 2 {
		t.Errorf("deleted = %v; want 2", got["deleted"])
	}
	if c, _ := env.store.GetCard("KEEP01"); c == nil {
		t.Error("protected card should survive Delete All")
	}
	if c, _ := env.store.GetCard("AAA111"); c != nil {
		t.Error("unprotected card should have been deleted")
	}
}

func TestPublicStylesEndpoints(t *testing.T) {
	env := newTestEnv(t)

	pubID, err := env.store.CreateStyle("Ocean Public", map[string]string{"page-bg": "#08191f"}, "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	privID, err := env.store.CreateStyle("Secret", map[string]string{"page-bg": "#111"}, "", "", false)
	if err != nil {
		t.Fatal(err)
	}

	// The public list contains only the Public theme.
	resp := env.get(t, "/api/styles/public")
	styles, ok := decodeBody(t, resp)["styles"].([]any)
	if !ok || len(styles) != 1 {
		t.Fatalf("public list = %v; want 1 style", styles)
	}
	if styles[0].(map[string]any)["name"] != "Ocean Public" {
		t.Errorf("public style = %v; want Ocean Public", styles[0])
	}

	// Public CSS: 200 for the Public theme, 404 for the Private one (can't fetch by id).
	resp = env.get(t, fmt.Sprintf("/api/styles/public/%d", pubID))
	if resp.StatusCode != http.StatusOK {
		t.Errorf("public css = %d; want 200", resp.StatusCode)
	}
	css, _ := decodeBody(t, resp)["css"].(string)
	if css == "" {
		t.Error("public css should be non-empty")
	}
	resp = env.get(t, fmt.Sprintf("/api/styles/public/%d", privID))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("private css by id = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCustomCardCostSetting(t *testing.T) {
	env := newTestEnv(t)

	// Public GET exposes the setting (with its default).
	settings, ok := decodeBody(t, env.get(t, "/api/settings"))["settings"].(map[string]any)
	if !ok {
		t.Fatal("settings missing from response")
	}
	if _, present := settings["custom_card_cost"]; !present {
		t.Error("custom_card_cost missing from public settings")
	}

	env.loginAdmin(t)

	// Valid value saves.
	resp := env.postJSON(t, "/api/settings", map[string]any{"settings": map[string]string{"custom_card_cost": "5000000"}})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("valid cost = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Non-numeric and negative values are rejected.
	for _, bad := range []string{"abc", "-5"} {
		resp = env.postJSON(t, "/api/settings", map[string]any{"settings": map[string]string{"custom_card_cost": bad}})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("cost %q = %d; want 400", bad, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
