package server_test

import (
	"fmt"
	"net/http"
	"testing"
)

// ── Garapon admin CRUD ──────────────────────────────────────────────────────

func TestGarapons_RequiresAuth(t *testing.T) {
	env := newTestEnv(t)

	if resp := env.get(t, "/api/garapons"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("list status = %d; want 401", resp.StatusCode)
		resp.Body.Close()
	}
	resp := env.postJSON(t, "/api/garapons", map[string]any{"action": "create", "title": "X"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("create status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

// createGarapon posts a minimal valid create and returns the new garapon's id.
func (e *testEnv) createGarapon(t *testing.T, title string) int {
	t.Helper()
	resp := e.postJSON(t, "/api/garapons", map[string]any{
		"action":  "create",
		"title":   title,
		"details": "round and round",
		"prizes": []map[string]any{
			{"name": "Grand", "ball_color": "#e5b53f", "rate": 1, "is_grand": true},
			{"name": "Consolation", "ball_color": "#cccccc", "rate": 9},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create garapon status = %d; want 201", resp.StatusCode)
	}
	g := decodeBody(t, resp)["garapon"].(map[string]any)
	if g["status"] != "open" {
		t.Errorf("new garapon status = %v; want open", g["status"])
	}
	return int(g["id"].(float64))
}

func TestGarapons_CreateListDetail(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	id := env.createGarapon(t, "Festival Drum")

	// List shows it.
	list := decodeBody(t, env.get(t, "/api/garapons"))["garapons"].([]any)
	if len(list) != 1 {
		t.Fatalf("expected 1 garapon, got %d", len(list))
	}

	// Detail carries garapon + (empty) players + draws.
	detail := decodeBody(t, env.get(t, fmt.Sprintf("/api/garapons/%d", id)))
	g := detail["garapon"].(map[string]any)
	if g["title"] != "Festival Drum" {
		t.Errorf("title = %v; want Festival Drum", g["title"])
	}
	prizes := g["prizes"].([]any)
	if len(prizes) != 2 {
		t.Errorf("expected 2 prizes, got %d", len(prizes))
	}
	if _, ok := detail["players"]; !ok {
		t.Error("detail missing players key")
	}
	if _, ok := detail["draws"]; !ok {
		t.Error("detail missing draws key")
	}
}

func TestGarapons_CreateValidation(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"missing title", map[string]any{"action": "create", "title": "  "}},
		{"no usable prizes", map[string]any{"action": "create", "title": "T", "prizes": []map[string]any{{"name": " "}}}},
		{"multiple grand", map[string]any{"action": "create", "title": "T", "prizes": []map[string]any{
			{"name": "A", "is_grand": true}, {"name": "B", "is_grand": true},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := env.postJSON(t, "/api/garapons", tc.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d; want 400", resp.StatusCode)
			}
			resp.Body.Close()
		})
	}
}

func TestGarapons_DefaultsSingleGrand(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// No prize flagged grand → the first row is promoted.
	resp := env.postJSON(t, "/api/garapons", map[string]any{
		"action": "create", "title": "T",
		"prizes": []map[string]any{{"name": "A", "rate": 1}, {"name": "B", "rate": 1}},
	})
	id := int(decodeBody(t, resp)["garapon"].(map[string]any)["id"].(float64))

	detail := decodeBody(t, env.get(t, fmt.Sprintf("/api/garapons/%d", id)))
	prizes := detail["garapon"].(map[string]any)["prizes"].([]any)
	first := prizes[0].(map[string]any)
	if first["is_grand"] != true {
		t.Errorf("first prize is_grand = %v; want true (auto-promoted)", first["is_grand"])
	}
}

func TestGarapons_Update(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createGarapon(t, "Old Title")

	resp := env.postJSON(t, "/api/garapons", map[string]any{
		"action": "update", "id": id, "title": "New Title",
		"prizes": []map[string]any{{"name": "Only", "rate": 1, "is_grand": true}},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("update status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	detail := decodeBody(t, env.get(t, fmt.Sprintf("/api/garapons/%d", id)))
	if got := detail["garapon"].(map[string]any)["title"]; got != "New Title" {
		t.Errorf("title = %v; want New Title", got)
	}
}

func TestGarapons_SetStatus(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createGarapon(t, "G")

	resp := env.postJSON(t, "/api/garapons", map[string]any{"action": "set_status", "id": id, "status": "closed"})
	if resp.StatusCode != 200 {
		t.Fatalf("set_status status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
	if got := decodeBody(t, env.get(t, fmt.Sprintf("/api/garapons/%d", id)))["garapon"].(map[string]any)["status"]; got != "closed" {
		t.Errorf("status = %v; want closed", got)
	}

	// Invalid status value is rejected.
	resp = env.postJSON(t, "/api/garapons", map[string]any{"action": "set_status", "id": id, "status": "paused"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestGarapons_Delete(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createGarapon(t, "G")

	resp := env.postJSON(t, "/api/garapons", map[string]any{"action": "delete", "id": id})
	if resp.StatusCode != 200 {
		t.Fatalf("delete status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	if resp := env.get(t, fmt.Sprintf("/api/garapons/%d", id)); resp.StatusCode != http.StatusNotFound {
		t.Errorf("detail after delete = %d; want 404", resp.StatusCode)
		resp.Body.Close()
	}
}

func TestGarapons_InvalidAction(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	resp := env.postJSON(t, "/api/garapons", map[string]any{"action": "explode"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestGarapon_DetailNotFound(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	if resp := env.get(t, "/api/garapons/9999"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d; want 404", resp.StatusCode)
		resp.Body.Close()
	}
}

// ── Garapon drawing links (players) ─────────────────────────────────────────

// createGaraponPlayer issues a drawing link and returns its token + id.
func (e *testEnv) createGaraponPlayer(t *testing.T, garaponID int, name string, maxDraws int) (token string, id int) {
	t.Helper()
	resp := e.postJSON(t, fmt.Sprintf("/api/garapons/%d/players", garaponID), map[string]any{
		"action": "create_player", "player_name": name, "max_draws": maxDraws,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create_player status = %d; want 201", resp.StatusCode)
	}
	p := decodeBody(t, resp)["player"].(map[string]any)
	return p["token"].(string), int(p["id"].(float64))
}

func TestGaraponPlayers_CreateAndDelete(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid := env.createGarapon(t, "G")

	_, pid := env.createGaraponPlayer(t, gid, "Hero", 2)

	// Detail now lists the link.
	players := decodeBody(t, env.get(t, fmt.Sprintf("/api/garapons/%d", gid)))["players"].([]any)
	if len(players) != 1 {
		t.Fatalf("expected 1 player, got %d", len(players))
	}

	// An undrawn link deletes cleanly while open.
	resp := env.postJSON(t, fmt.Sprintf("/api/garapons/%d/players", gid), map[string]any{
		"action": "delete_player", "player_id": pid,
	})
	if resp.StatusCode != 200 {
		t.Fatalf("delete_player status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestGaraponPlayers_DeleteDrawnWhileOpenConflicts(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid := env.createGarapon(t, "G")
	token, pid := env.createGaraponPlayer(t, gid, "Hero", 2)

	// Use one draw, then deleting the link while the garapon is open must 409.
	env.postJSON(t, "/api/garapon/"+token+"/draw", map[string]any{}).Body.Close()

	resp := env.postJSON(t, fmt.Sprintf("/api/garapons/%d/players", gid), map[string]any{
		"action": "delete_player", "player_id": pid,
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("delete drawn (open) status = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()

	// Closing the garapon allows the cleanup; the draw stays in the log.
	env.postJSON(t, "/api/garapons", map[string]any{"action": "set_status", "id": gid, "status": "closed"}).Body.Close()
	resp = env.postJSON(t, fmt.Sprintf("/api/garapons/%d/players", gid), map[string]any{
		"action": "delete_player", "player_id": pid,
	})
	if resp.StatusCode != 200 {
		t.Errorf("delete drawn (closed) status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	draws := decodeBody(t, env.get(t, fmt.Sprintf("/api/garapons/%d", gid)))["draws"].([]any)
	if len(draws) != 1 {
		t.Errorf("expected 1 retained draw after link deletion, got %d", len(draws))
	}
}

func TestGaraponPlayers_DeleteUnknown(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid := env.createGarapon(t, "G")
	resp := env.postJSON(t, fmt.Sprintf("/api/garapons/%d/players", gid), map[string]any{
		"action": "delete_player", "player_id": 9999,
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

// ── Garapon public view + draw ──────────────────────────────────────────────

func TestGaraponPublic_View(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid := env.createGarapon(t, "Festival")
	token, _ := env.createGaraponPlayer(t, gid, "Hero", 3)

	// The public view needs no auth — use a fresh, cookie-less client.
	env.client.Jar = nil
	data := decodeBody(t, env.get(t, "/api/garapon/"+token))
	g := data["garapon"].(map[string]any)
	if g["title"] != "Festival" {
		t.Errorf("title = %v; want Festival", g["title"])
	}
	// Odds are stripped from the public payload.
	for _, p := range g["prizes"].([]any) {
		if rate := p.(map[string]any)["rate"].(float64); rate != 0 {
			t.Errorf("public prize rate = %v; want 0 (odds hidden)", rate)
		}
	}
	player := data["player"].(map[string]any)
	if player["max_draws"].(float64) != 3 {
		t.Errorf("max_draws = %v; want 3", player["max_draws"])
	}
}

func TestGaraponPublic_UnknownToken(t *testing.T) {
	env := newTestEnv(t)
	if resp := env.get(t, "/api/garapon/deadbeef"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d; want 404", resp.StatusCode)
		resp.Body.Close()
	}
}

func TestGaraponDraw_Success(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid := env.createGarapon(t, "G")
	token, _ := env.createGaraponPlayer(t, gid, "Hero", 2)

	data := decodeBody(t, env.postJSON(t, "/api/garapon/"+token+"/draw", map[string]any{}))
	if data["draws_used"].(float64) != 1 {
		t.Errorf("draws_used = %v; want 1", data["draws_used"])
	}
	if data["draw"] == nil {
		t.Error("expected a draw in the response")
	}
}

func TestGaraponDraw_NoDrawsRemaining(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid := env.createGarapon(t, "G")
	token, _ := env.createGaraponPlayer(t, gid, "Hero", 1)

	env.postJSON(t, "/api/garapon/"+token+"/draw", map[string]any{}).Body.Close() // uses the only draw
	resp := env.postJSON(t, "/api/garapon/"+token+"/draw", map[string]any{})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestGaraponDraw_Closed(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	gid := env.createGarapon(t, "G")
	token, _ := env.createGaraponPlayer(t, gid, "Hero", 2)
	env.postJSON(t, "/api/garapons", map[string]any{"action": "set_status", "id": gid, "status": "closed"}).Body.Close()

	resp := env.postJSON(t, "/api/garapon/"+token+"/draw", map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400 (closed)", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestGaraponDraw_UnknownToken(t *testing.T) {
	env := newTestEnv(t)
	resp := env.postJSON(t, "/api/garapon/deadbeef/draw", map[string]any{})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()
}
