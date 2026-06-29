package server_test

import (
	"fmt"
	"net/http"
	"testing"

	"app-suite/internal/auth"
)

// ── Stamp Rally admin CRUD ──────────────────────────────────────────────────

func TestStampRally_RequiresAuth(t *testing.T) {
	env := newTestEnv(t)
	if resp := env.get(t, "/api/stamp-rallies"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("list status = %d; want 401", resp.StatusCode)
		resp.Body.Close()
	}
	resp := env.postJSON(t, "/api/stamp-rallies", map[string]any{"action": "create", "title": "X"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("create status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

// createRally posts a create with one stamp (password "alpha") and one prize, and
// returns the new rally's id. The stamp is paused per the `paused` arg.
func (e *testEnv) createRally(t *testing.T, title string, paused bool) int {
	t.Helper()
	resp := e.postJSON(t, "/api/stamp-rallies", map[string]any{
		"action": "create",
		"title":  title,
		"stamps": []map[string]any{
			{
				"image":     "images/stamp_stamps/a.png",
				"password":  "alpha",
				"paused":    paused,
				"placement": map[string]any{"x": 10, "y": 10, "width": 15, "height": 15},
			},
		},
		"prizes": []map[string]any{
			{"name": "Trophy", "image": "images/stamp_prizes/t.png",
				"placement": map[string]any{"x": 70, "y": 20, "width": 20, "height": 20}},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create rally status = %d; want 201", resp.StatusCode)
	}
	r := decodeBody(t, resp)["stamp_rally"].(map[string]any)
	return int(r["id"].(float64))
}

// issueCard issues a participant card on a rally and returns its token.
func (e *testEnv) issueCard(t *testing.T, rallyID int, name string) string {
	t.Helper()
	resp := e.postJSON(t, fmt.Sprintf("/api/stamp-rallies/%d/cards", rallyID), map[string]any{
		"action": "create_card", "participant_name": name,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create_card status = %d; want 201", resp.StatusCode)
	}
	return decodeBody(t, resp)["card"].(map[string]any)["token"].(string)
}

func TestStampRally_CreateListDetail(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	id := env.createRally(t, "Summer Rally", false)

	list := decodeBody(t, env.get(t, "/api/stamp-rallies"))["stamp_rallies"].([]any)
	if len(list) != 1 {
		t.Fatalf("expected 1 rally, got %d", len(list))
	}
	detail := decodeBody(t, env.get(t, fmt.Sprintf("/api/stamp-rallies/%d", id)))
	r := detail["stamp_rally"].(map[string]any)
	if len(r["stamps"].([]any)) != 1 || len(r["prizes"].([]any)) != 1 {
		t.Errorf("detail stamps/prizes counts wrong: %v", r)
	}
	if _, ok := detail["cards"]; !ok {
		t.Error("detail missing cards key")
	}
}

func TestStampRally_CreateValidation(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	resp := env.postJSON(t, "/api/stamp-rallies", map[string]any{"action": "create", "title": "  "})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing title status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// ── Public flow (token view + password collection + completion) ─────────────

func TestStampCard_PublicFlowAndCompletion(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRally(t, "Rally", false)
	token := env.issueCard(t, id, "Tataru")

	// Public view: no passwords leak; one uncollected stamp; prizes not revealed.
	card := decodeBody(t, env.get(t, "/api/stamp-card/"+token))
	stamps := card["stamps"].([]any)
	if len(stamps) != 1 {
		t.Fatalf("public stamps = %d; want 1", len(stamps))
	}
	if _, leaked := stamps[0].(map[string]any)["password"]; leaked {
		t.Error("public stamp leaked its password")
	}
	if card["prizes_revealed"] != false {
		t.Error("prizes revealed before completion")
	}

	// Wrong password → 400.
	resp := env.postJSON(t, "/api/stamp-card/"+token+"/stamp", map[string]any{"password": "nope"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("wrong password status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Correct password → collected; the only stamp → card completes + prizes reveal.
	resp = env.postJSON(t, "/api/stamp-card/"+token+"/stamp", map[string]any{"password": "alpha"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("collect status = %d; want 200", resp.StatusCode)
	}
	body := decodeBody(t, resp)
	got := body["card"].(map[string]any)
	if got["completed"] != true || got["prizes_revealed"] != true {
		t.Errorf("after final collect: completed=%v revealed=%v; want true,true", got["completed"], got["prizes_revealed"])
	}
	prize := got["prizes"].([]any)[0].(map[string]any)
	if prize["name"] != "Trophy" {
		t.Errorf("revealed prize name = %v; want Trophy", prize["name"])
	}

	// Re-collecting the same stamp → 409.
	resp = env.postJSON(t, "/api/stamp-card/"+token+"/stamp", map[string]any{"password": "alpha"})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("re-collect status = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()

	// Logs show the collection with participant + default stall name.
	logs := decodeBody(t, env.get(t, fmt.Sprintf("/api/stamp-rallies/%d/logs", id)))["logs"].([]any)
	if len(logs) != 1 {
		t.Fatalf("logs = %d; want 1", len(logs))
	}
	row := logs[0].(map[string]any)
	if row["participant_name"] != "Tataru" || row["stall_name"] != "Senpan Tea House" {
		t.Errorf("log row = %v; want Tataru / Senpan Tea House", row)
	}
}

func TestStampCard_ClosedStall(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRally(t, "Paused Rally", true) // stamp is paused
	token := env.issueCard(t, id, "Borin")

	resp := env.postJSON(t, "/api/stamp-card/"+token+"/stamp", map[string]any{"password": "alpha"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("paused-stall collect status = %d; want 400", resp.StatusCode)
	}
	if msg := decodeBody(t, resp)["error"].(string); msg != "This stall is currently closed and cannot be stamped" {
		t.Errorf("closed-stall message = %q", msg)
	}
}

func TestStampCard_UnknownToken(t *testing.T) {
	env := newTestEnv(t)
	resp := env.get(t, "/api/stamp-card/deadbeef")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown token status = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

// setRallyStatus posts a status change for a rally.
func (e *testEnv) setRallyStatus(t *testing.T, rallyID int, status string) {
	t.Helper()
	resp := e.postJSON(t, "/api/stamp-rallies", map[string]any{"action": "set_status", "id": rallyID, "status": status})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set_status %s = %d; want 200", status, resp.StatusCode)
	}
	resp.Body.Close()
}

// TestStampRally_CloseKeepsLogs verifies a closed rally is read-only, that a card with
// stamps can only be deleted once the rally is closed, and that deletion preserves the
// View Logs entries (via the participant/stall snapshots).
func TestStampRally_CloseKeepsLogs(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createRally(t, "Festival", false)
	token := env.issueCard(t, id, "Tester")

	// Collect the stamp, then find the card id (the only card).
	if resp := env.postJSON(t, "/api/stamp-card/"+token+"/stamp", map[string]any{"password": "alpha"}); resp.StatusCode != http.StatusOK {
		t.Fatalf("collect = %d; want 200", resp.StatusCode)
		resp.Body.Close()
	}
	cards := decodeBody(t, env.get(t, fmt.Sprintf("/api/stamp-rallies/%d", id)))["cards"].([]any)
	cardID := int(cards[0].(map[string]any)["id"].(float64))

	// While open, deleting a card that has stamps is rejected (409).
	resp := env.postJSON(t, fmt.Sprintf("/api/stamp-rallies/%d/cards", id), map[string]any{"action": "delete_card", "card_id": cardID})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("delete collected card (open) = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()

	// Close the rally → stamping is now blocked (read-only)...
	env.setRallyStatus(t, id, "closed")
	resp = env.postJSON(t, "/api/stamp-card/"+token+"/stamp", map[string]any{"password": "alpha"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("collect on closed rally = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// ...and the card can now be deleted, but its log row survives.
	resp = env.postJSON(t, fmt.Sprintf("/api/stamp-rallies/%d/cards", id), map[string]any{"action": "delete_card", "card_id": cardID})
	if resp.StatusCode != http.StatusOK || decodeBody(t, resp)["deleted"] != true {
		t.Fatalf("delete collected card (closed) failed")
	}
	logs := decodeBody(t, env.get(t, fmt.Sprintf("/api/stamp-rallies/%d/logs", id)))["logs"].([]any)
	if len(logs) != 1 {
		t.Fatalf("logs after card delete = %d; want 1 (kept)", len(logs))
	}
	if logs[0].(map[string]any)["participant_name"] != "Tester" {
		t.Errorf("kept log lost its participant snapshot: %v", logs[0])
	}
}

// TestGarapon_LinkedStampRally verifies a garapon linked to an open rally auto-issues a
// stamp card sharing the SAME token when a drawing link is created, rejects linking a
// closed rally, and that deleting the rally clears the garapon's link.
func TestGarapon_LinkedStampRally(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	rallyID := env.createRally(t, "Linked Rally", false)

	// Create a garapon linked to the open rally.
	resp := env.postJSON(t, "/api/garapons", map[string]any{
		"action": "create", "title": "Drum", "stamp_rally_id": rallyID,
		"prizes": []map[string]any{{"name": "Grand", "ball_color": "#e5b53f", "rate": 1, "is_grand": true}},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create linked garapon = %d; want 201", resp.StatusCode)
	}
	gid := int(decodeBody(t, resp)["garapon"].(map[string]any)["id"].(float64))

	// Issue a drawing link → it also issues a stamp card with the SAME token.
	resp = env.postJSON(t, fmt.Sprintf("/api/garapons/%d/players", gid), map[string]any{
		"action": "create_player", "player_name": "Tester", "max_draws": 1,
	})
	player := decodeBody(t, resp)["player"].(map[string]any)
	token := player["token"].(string)
	if player["stamp_card_token"] != token {
		t.Errorf("stamp_card_token = %v; want same as drawing token %q", player["stamp_card_token"], token)
	}
	// The shared token resolves as a stamp card too.
	if r := env.get(t, "/api/stamp-card/"+token); r.StatusCode != http.StatusOK {
		t.Errorf("/stamp-card/<shared token> = %d; want 200", r.StatusCode)
		r.Body.Close()
	}

	// Linking a CLOSED rally is rejected.
	env.setRallyStatus(t, rallyID, "closed")
	resp = env.postJSON(t, "/api/garapons", map[string]any{
		"action": "create", "title": "Drum2", "stamp_rally_id": rallyID,
		"prizes": []map[string]any{{"name": "G", "rate": 1, "is_grand": true}},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("link closed rally = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Deleting the rally clears the first garapon's link.
	env.postJSON(t, "/api/stamp-rallies", map[string]any{"action": "delete", "id": rallyID}).Body.Close()
	g := decodeBody(t, env.get(t, fmt.Sprintf("/api/garapons/%d", gid)))["garapon"].(map[string]any)
	if g["stamp_rally_id"] != nil {
		t.Errorf("garapon link after rally delete = %v; want null", g["stamp_rally_id"])
	}
}

// TestStampRally_PermissionGating verifies the festival-stamp-rally page permission
// gates the admin API (store-driven, single session — see affiliates_test.go).
func TestStampRally_PermissionGating(t *testing.T) {
	env := newTestEnv(t)
	hash, err := auth.Hash("password123")
	if err != nil {
		t.Fatal(err)
	}
	staff, err := env.store.CreateUser("staff", hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.store.SetUserActive(staff.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := env.store.SetUserPermissions(staff.ID, []string{"bingo-cards"}); err != nil {
		t.Fatal(err)
	}
	env.postJSON(t, "/api/auth", map[string]string{
		"action": "login", "username": "staff", "password": "password123",
	}).Body.Close()

	if resp := env.get(t, "/api/stamp-rallies"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("wrong-permission status = %d; want 403", resp.StatusCode)
		resp.Body.Close()
	}
	if err := env.store.SetUserPermissions(staff.ID, []string{"festival-stamp-rally"}); err != nil {
		t.Fatal(err)
	}
	if resp := env.get(t, "/api/stamp-rallies"); resp.StatusCode != http.StatusOK {
		t.Errorf("granted status = %d; want 200", resp.StatusCode)
		resp.Body.Close()
	}
}
