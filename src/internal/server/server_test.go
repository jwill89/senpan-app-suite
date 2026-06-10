package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"app-suite/internal/server"
	"app-suite/internal/store"
	"app-suite/internal/ws"
)

const (
	testPassword = "test-password"
	testSecret   = "test-secret-key-for-sessions-32b"
)

// testEnv bundles a test server, authenticated client, and raw store.
type testEnv struct {
	ts     *httptest.Server
	client *http.Client
	store  *store.Store
	srv    *server.Server
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	hub := ws.NewHub()
	srv := server.New(st, hub, testSecret, testPassword, t.TempDir())

	ts := httptest.NewTLSServer(srv)
	t.Cleanup(ts.Close)

	jar, _ := cookiejar.New(nil)
	client := ts.Client()
	client.Jar = jar

	return &testEnv{ts: ts, client: client, store: st, srv: srv}
}

func (e *testEnv) url(path string) string {
	return e.ts.URL + path
}

func (e *testEnv) get(t *testing.T, path string) *http.Response {
	t.Helper()
	resp, err := e.client.Get(e.url(path))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func (e *testEnv) postJSON(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	resp, err := e.client.Post(e.url(path), "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func (e *testEnv) loginAdmin(t *testing.T) {
	t.Helper()
	resp := e.postJSON(t, "/api/auth", map[string]string{"action": "login", "password": testPassword})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("admin login failed: %d", resp.StatusCode)
	}
}

func decodeBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

// ── CORS ────────────────────────────────────────────────────────────────────

func TestCORS_Options(t *testing.T) {
	env := newTestEnv(t)

	req, _ := http.NewRequest("OPTIONS", env.url("/api/auth"), nil)
	req.Header.Set("Origin", "http://localhost")
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d; want 204", resp.StatusCode)
	}
	if h := resp.Header.Get("Access-Control-Allow-Methods"); h == "" {
		t.Error("missing CORS Allow-Methods header")
	}
}

func TestCORS_Headers(t *testing.T) {
	env := newTestEnv(t)

	req, _ := http.NewRequest("GET", env.url("/api/game"), nil)
	req.Header.Set("Origin", "http://localhost")
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if h := resp.Header.Get("Access-Control-Allow-Origin"); h != "http://localhost" {
		t.Errorf("CORS origin = %q; want http://localhost", h)
	}
}

// ── Auth ────────────────────────────────────────────────────────────────────

func TestAuth_CheckNotAuthenticated(t *testing.T) {
	env := newTestEnv(t)

	data := decodeBody(t, env.get(t, "/api/auth"))
	if data["authenticated"] != false {
		t.Errorf("expected authenticated=false, got %v", data["authenticated"])
	}
}

func TestAuth_LoginSuccess(t *testing.T) {
	env := newTestEnv(t)

	resp := env.postJSON(t, "/api/auth", map[string]string{"action": "login", "password": testPassword})
	data := decodeBody(t, resp)
	if data["success"] != true {
		t.Errorf("expected success=true, got %v", data)
	}

	// Should now be authenticated
	data = decodeBody(t, env.get(t, "/api/auth"))
	if data["authenticated"] != true {
		t.Error("expected authenticated=true after login")
	}
}

func TestAuth_LoginWrongPassword(t *testing.T) {
	env := newTestEnv(t)

	resp := env.postJSON(t, "/api/auth", map[string]string{"action": "login", "password": "wrong"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAuth_Logout(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/auth", map[string]string{"action": "logout"})
	resp.Body.Close()

	data := decodeBody(t, env.get(t, "/api/auth"))
	if data["authenticated"] != false {
		t.Error("expected authenticated=false after logout")
	}
}

// ── Board ───────────────────────────────────────────────────────────────────

func TestBoard_MissingID(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/board")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestBoard_NotFound(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/board?id=ZZZZZZ")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestBoard_Found(t *testing.T) {
	env := newTestEnv(t)

	board := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	if err := env.store.SaveCard("TEST01", board); err != nil {
		t.Fatal(err)
	}

	resp := env.get(t, "/api/board?id=TEST01")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	data := decodeBody(t, resp)

	if data["card"] == nil {
		t.Error("expected card in response")
	}
	if _, ok := data["game"]; !ok {
		t.Error("expected game key in response")
	}
	if _, ok := data["game_details"]; !ok {
		t.Error("expected game_details key in response")
	}
}

func TestBoard_Preview(t *testing.T) {
	env := newTestEnv(t)

	board := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	if err := env.store.SaveCard("PREV01", board); err != nil {
		t.Fatal(err)
	}

	resp := env.get(t, "/api/board?id=PREV01&preview=1")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	data := decodeBody(t, resp)

	if data["card"] == nil {
		t.Error("expected card in preview")
	}
	if data["game"] != nil {
		t.Error("preview should not include game")
	}
}

func TestBoard_CaseInsensitive(t *testing.T) {
	env := newTestEnv(t)

	board := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	if err := env.store.SaveCard("UPPER1", board); err != nil {
		t.Fatal(err)
	}

	resp := env.get(t, "/api/board?id=upper1")
	if resp.StatusCode != 200 {
		t.Errorf("status = %d; want 200 for lowercase lookup", resp.StatusCode)
	}
	resp.Body.Close()
}

// ── Cards (admin-only) ─────────────────────────────────────────────────────

func TestCards_RequiresAuth(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/cards")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /api/cards status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()

	resp = env.postJSON(t, "/api/cards", map[string]any{"action": "generate", "count": 1})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("POST /api/cards status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCards_Generate(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/cards", map[string]any{"action": "generate", "count": 3})
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d; body = %s", resp.StatusCode, body)
	}
	data := decodeBody(t, resp)

	count, _ := data["count"].(float64)
	if count != 3 {
		t.Errorf("count = %v; want 3", data["count"])
	}

	cards, _ := data["cards"].([]any)
	if len(cards) != 3 {
		t.Errorf("expected 3 cards, got %d", len(cards))
	}

	// Each card should have an ID and board_data
	for i, c := range cards {
		card, ok := c.(map[string]any)
		if !ok {
			t.Fatalf("card %d: unexpected type", i)
		}
		id, _ := card["id"].(string)
		if len(id) != 6 {
			t.Errorf("card %d: ID length = %d; want 6", i, len(id))
		}
	}
}

func TestCards_List(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Generate cards first
	env.postJSON(t, "/api/cards", map[string]any{"action": "generate", "count": 2}).Body.Close()

	resp := env.get(t, "/api/cards")
	data := decodeBody(t, resp)

	cards, _ := data["cards"].([]any)
	if len(cards) != 2 {
		t.Errorf("expected 2 cards, got %d", len(cards))
	}
}

func TestCards_Delete(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Generate a card
	resp := env.postJSON(t, "/api/cards", map[string]any{"action": "generate", "count": 1})
	data := decodeBody(t, resp)
	cards := data["cards"].([]any)
	cardID := cards[0].(map[string]any)["id"].(string)

	// Delete it
	resp = env.postJSON(t, "/api/cards", map[string]any{"action": "delete", "id": cardID})
	data = decodeBody(t, resp)
	if data["deleted"] != true {
		t.Errorf("expected deleted=true, got %v", data["deleted"])
	}

	// Should not exist anymore
	resp = env.get(t, "/api/board?id="+cardID)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("deleted card should 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCards_DeleteAll(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/cards", map[string]any{"action": "generate", "count": 5}).Body.Close()

	resp := env.postJSON(t, "/api/cards", map[string]any{"action": "delete_all"})
	data := decodeBody(t, resp)
	deleted, _ := data["deleted"].(float64)
	if deleted != 5 {
		t.Errorf("expected 5 deleted, got %v", data["deleted"])
	}

	// List should be empty
	resp = env.get(t, "/api/cards")
	data = decodeBody(t, resp)
	cards, _ := data["cards"].([]any)
	if len(cards) != 0 {
		t.Errorf("expected 0 cards, got %d", len(cards))
	}
}

func TestCards_DeleteMissing(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/cards", map[string]any{"action": "delete", "id": "ZZZZZZ"})
	data := decodeBody(t, resp)
	if data["deleted"] != false {
		t.Errorf("expected deleted=false for missing card, got %v", data["deleted"])
	}
}

func TestCards_InvalidAction(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/cards", map[string]any{"action": "explode"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCards_GenerateClampedCount(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Count > 500 should be clamped
	resp := env.postJSON(t, "/api/cards", map[string]any{"action": "generate", "count": 9999})
	data := decodeBody(t, resp)
	count, _ := data["count"].(float64)
	if count != 500 {
		t.Errorf("expected clamped count 500, got %v", count)
	}
}

// ── Patterns (admin-only) ──────────────────────────────────────────────────

func TestPatterns_RequiresAuth(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/patterns")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /api/patterns status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func testPattern5x5() [][]bool {
	grid := make([][]bool, 5)
	for i := range grid {
		grid[i] = make([]bool, 5)
	}
	for c := 0; c < 5; c++ {
		grid[0][c] = true
	}
	return grid
}

// testPattern5x5Alt returns a different 5×5 pattern (bottom row) to avoid
// duplicate detection when a test needs two distinct patterns.
func testPattern5x5Alt() [][]bool {
	grid := make([][]bool, 5)
	for i := range grid {
		grid[i] = make([]bool, 5)
	}
	for c := 0; c < 5; c++ {
		grid[4][c] = true
	}
	return grid
}

func TestPatterns_Create(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/patterns", map[string]any{
		"action":       "create",
		"name":         "Top Row",
		"pattern_data": testPattern5x5(),
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d; body = %s", resp.StatusCode, body)
	}
	data := decodeBody(t, resp)
	pat, _ := data["pattern"].(map[string]any)
	if pat["name"] != "Top Row" {
		t.Errorf("name = %v; want Top Row", pat["name"])
	}
}

func TestPatterns_Create_InvalidGrid(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Wrong dimensions
	resp := env.postJSON(t, "/api/patterns", map[string]any{
		"action":       "create",
		"name":         "Bad",
		"pattern_data": [][]bool{{true}},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPatterns_Create_EmptyName(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/patterns", map[string]any{
		"action":       "create",
		"name":         "",
		"pattern_data": testPattern5x5(),
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPatterns_List(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/patterns", map[string]any{
		"action": "create", "name": "Pat A", "pattern_data": testPattern5x5(),
	}).Body.Close()
	env.postJSON(t, "/api/patterns", map[string]any{
		"action": "create", "name": "Pat B", "pattern_data": testPattern5x5Alt(),
	}).Body.Close()

	resp := env.get(t, "/api/patterns")
	data := decodeBody(t, resp)
	patterns, _ := data["patterns"].([]any)
	if len(patterns) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(patterns))
	}
}

func TestPatterns_Rename(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/patterns", map[string]any{
		"action": "create", "name": "Old", "pattern_data": testPattern5x5(),
	})
	data := decodeBody(t, resp)
	pat := data["pattern"].(map[string]any)
	id := pat["id"].(float64)

	resp = env.postJSON(t, "/api/patterns", map[string]any{
		"action": "rename", "id": id, "name": "New",
	})
	data = decodeBody(t, resp)
	if data["renamed"] != true {
		t.Errorf("expected renamed=true, got %v", data["renamed"])
	}
}

func TestPatterns_Delete(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/patterns", map[string]any{
		"action": "create", "name": "Temp", "pattern_data": testPattern5x5(),
	})
	data := decodeBody(t, resp)
	pat := data["pattern"].(map[string]any)
	id := pat["id"].(float64)

	resp = env.postJSON(t, "/api/patterns", map[string]any{
		"action": "delete", "id": id,
	})
	data = decodeBody(t, resp)
	if data["deleted"] != true {
		t.Errorf("expected deleted=true, got %v", data["deleted"])
	}
}

func TestPatterns_Reorder(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/patterns", map[string]any{
		"action": "create", "name": "A", "pattern_data": testPattern5x5(),
	}).Body.Close()
	resp := env.postJSON(t, "/api/patterns", map[string]any{
		"action": "create", "name": "B", "pattern_data": testPattern5x5Alt(),
	})
	data := decodeBody(t, resp)
	idB := data["pattern"].(map[string]any)["id"].(float64)

	resp = env.postJSON(t, "/api/patterns", map[string]any{
		"action": "reorder", "id": idB, "direction": "up",
	})
	data = decodeBody(t, resp)
	patterns, _ := data["patterns"].([]any)
	if len(patterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d", len(patterns))
	}
	first := patterns[0].(map[string]any)
	if first["name"] != "B" {
		t.Errorf("first pattern should be B after move up, got %v", first["name"])
	}
}

func TestPatterns_InvalidAction(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/patterns", map[string]any{"action": "explode"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// ── Game ────────────────────────────────────────────────────────────────────

func TestGame_State_NoActive(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/game")
	data := decodeBody(t, resp)

	if data["game"] != nil {
		t.Errorf("expected null game, got %v", data["game"])
	}
}

func TestGame_RequiresAuth(t *testing.T) {
	env := newTestEnv(t)

	resp := env.postJSON(t, "/api/game", map[string]any{"action": "start", "pattern_ids": []int{1}})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestGame_Start_NoPatterns(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/game", map[string]any{"action": "start", "pattern_ids": []int{}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestGame_FullLifecycle(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Create a pattern
	resp := env.postJSON(t, "/api/patterns", map[string]any{
		"action": "create", "name": "Test", "pattern_data": testPattern5x5(),
	})
	data := decodeBody(t, resp)
	patID := data["pattern"].(map[string]any)["id"].(float64)

	// Start game
	resp = env.postJSON(t, "/api/game", map[string]any{
		"action": "start", "pattern_ids": []int{int(patID)},
	})
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("start game: status = %d; body = %s", resp.StatusCode, body)
	}
	data = decodeBody(t, resp)
	game, _ := data["game"].(map[string]any)
	if game["id"] == nil {
		t.Error("expected game id")
	}

	// GET game state should return active game
	resp = env.get(t, "/api/game")
	data = decodeBody(t, resp)
	if data["game"] == nil {
		t.Error("expected active game from GET")
	}

	// Draw a number
	resp = env.postJSON(t, "/api/game", map[string]any{"action": "draw"})
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("draw: status = %d; body = %s", resp.StatusCode, body)
	}
	data = decodeBody(t, resp)
	drawn, _ := data["drawn"].(map[string]any)
	if drawn == nil {
		t.Fatal("expected drawn object")
	}
	num, _ := drawn["number"].(float64)
	if num < 1 || num > 75 {
		t.Errorf("drawn number = %v; want 1–75", num)
	}
	letter, _ := drawn["letter"].(string)
	if letter == "" {
		t.Error("expected non-empty letter")
	}
	callOrder, _ := drawn["call_order"].(float64)
	if callOrder != 1 {
		t.Errorf("call_order = %v; want 1", callOrder)
	}

	// End game
	resp = env.postJSON(t, "/api/game", map[string]any{"action": "end"})
	data = decodeBody(t, resp)
	if data["ended"] != true {
		t.Errorf("expected ended=true, got %v", data["ended"])
	}

	// End again — should return false
	resp = env.postJSON(t, "/api/game", map[string]any{"action": "end"})
	data = decodeBody(t, resp)
	if data["ended"] != false {
		t.Errorf("expected ended=false on second end, got %v", data["ended"])
	}
}

func TestGame_Draw_NoActive(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/game", map[string]any{"action": "draw"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestGame_InvalidAction(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/game", map[string]any{"action": "explode"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestGame_StateWithActiveGame(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Create pattern and start game
	resp := env.postJSON(t, "/api/patterns", map[string]any{
		"action": "create", "name": "Test", "pattern_data": testPattern5x5(),
	})
	data := decodeBody(t, resp)
	patID := data["pattern"].(map[string]any)["id"].(float64)

	env.postJSON(t, "/api/game", map[string]any{
		"action": "start", "pattern_ids": []int{int(patID)},
	}).Body.Close()

	// GET game state (no auth required)
	env2 := &testEnv{ts: env.ts}
	jar2, _ := cookiejar.New(nil)
	env2.client = env.ts.Client()
	env2.client.Jar = jar2

	resp = env2.get(t, "/api/game")
	data = decodeBody(t, resp)
	if data["game"] == nil {
		t.Error("expected game state from unauthenticated GET")
	}
}

func TestGame_BoardWithActiveGame(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Create a card
	board := [][]int{
		{1, 16, 31, 46, 61},
		{2, 17, 32, 47, 62},
		{3, 18, 0, 48, 63},
		{4, 19, 34, 49, 64},
		{5, 20, 35, 50, 65},
	}
	if err := env.store.SaveCard("GAME01", board); err != nil {
		t.Fatal(err)
	}

	// Create pattern and start game
	resp := env.postJSON(t, "/api/patterns", map[string]any{
		"action": "create", "name": "Test", "pattern_data": testPattern5x5(),
	})
	data := decodeBody(t, resp)
	patID := data["pattern"].(map[string]any)["id"].(float64)

	env.postJSON(t, "/api/game", map[string]any{
		"action": "start", "pattern_ids": []int{int(patID)},
	}).Body.Close()

	// Draw a number
	env.postJSON(t, "/api/game", map[string]any{"action": "draw"}).Body.Close()

	// Board should include game state with called numbers
	resp = env.get(t, "/api/board?id=GAME01")
	data = decodeBody(t, resp)
	game, _ := data["game"].(map[string]any)
	if game == nil {
		t.Fatal("expected game in board response")
	}
	called, _ := game["called_numbers"].([]any)
	if len(called) != 1 {
		t.Errorf("expected 1 called number, got %d", len(called))
	}
}

// ── JSON error format ───────────────────────────────────────────────────────

func TestErrorFormat(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/board")
	data := decodeBody(t, resp)

	if _, ok := data["error"]; !ok {
		t.Error("expected error key in JSON error response")
	}
}

func TestJSONContentType(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/game")
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q; want application/json; charset=utf-8", ct)
	}
}

func TestCacheControlNoStore(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/game")
	defer resp.Body.Close()

	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q; want no-store", cc)
	}
}

// ── Raffles ─────────────────────────────────────────────────────────────────

func TestRaffles_RequiresAuth(t *testing.T) {
	env := newTestEnv(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{"action": "create", "title": "Test"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRaffles_Create(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action":         "create",
		"title":          "Prize Raffle",
		"description":    "Win big!",
		"max_entries":    5,
		"cost_per_entry": 100,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d; want 201", resp.StatusCode)
	}
	data := decodeBody(t, resp)
	raffle, _ := data["raffle"].(map[string]any)
	if raffle == nil {
		t.Fatal("expected raffle in response")
	}
	if raffle["title"] != "Prize Raffle" {
		t.Errorf("title = %v; want Prize Raffle", raffle["title"])
	}
	if raffle["status"] != "open" {
		t.Errorf("status = %v; want open", raffle["status"])
	}
}

func TestRaffles_CreateMissingTitle(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRaffles_List(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "R1", "max_entries": 1,
	}).Body.Close()
	env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "R2", "max_entries": 1,
	}).Body.Close()

	resp := env.get(t, "/api/raffles")
	data := decodeBody(t, resp)
	raffles, _ := data["raffles"].([]any)
	if len(raffles) != 2 {
		t.Errorf("expected 2 raffles, got %d", len(raffles))
	}
}

func TestRaffles_Update(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Original", "max_entries": 1,
	})
	data := decodeBody(t, resp)
	raffle := data["raffle"].(map[string]any)
	id := raffle["id"].(float64)

	resp = env.postJSON(t, "/api/raffles", map[string]any{
		"action": "update", "id": id, "title": "Updated", "max_entries": 10,
	})
	data = decodeBody(t, resp)
	if data["ok"] != true {
		t.Errorf("expected ok=true, got %v", data["ok"])
	}
}

func TestRaffles_Delete(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "To Delete", "max_entries": 1,
	})
	data := decodeBody(t, resp)
	raffle := data["raffle"].(map[string]any)
	id := raffle["id"].(float64)

	resp = env.postJSON(t, "/api/raffles", map[string]any{
		"action": "delete", "id": id,
	})
	data = decodeBody(t, resp)
	if data["deleted"] != true {
		t.Errorf("expected deleted=true, got %v", data["deleted"])
	}
}

func TestRaffles_InvalidAction(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{"action": "explode"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRaffles_Detail(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Detail Test", "max_entries": 5, "cost_per_entry": 100,
	})
	data := decodeBody(t, resp)
	raffle := data["raffle"].(map[string]any)
	id := int(raffle["id"].(float64))

	resp = env.get(t, fmt.Sprintf("/api/raffles/%d", id))
	data = decodeBody(t, resp)
	r, _ := data["raffle"].(map[string]any)
	if r == nil {
		t.Fatal("expected raffle in detail response")
	}
	if r["title"] != "Detail Test" {
		t.Errorf("title = %v; want Detail Test", r["title"])
	}
	// Admin should get entries array
	entries, _ := data["entries"].([]any)
	if entries == nil {
		t.Error("expected entries array for admin")
	}
}

func TestRaffles_DetailNotFound(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/raffles/99999")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRaffles_Enter(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Create raffle
	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Enter Test", "max_entries": 5, "cost_per_entry": 100,
		"signup_instructions": "Pay the gil!",
	})
	data := decodeBody(t, resp)
	raffle := data["raffle"].(map[string]any)
	id := int(raffle["id"].(float64))

	// Enter as player (no auth needed for enter)
	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", id), map[string]any{
		"character_name": "TestPlayer", "world": "Excalibur", "num_entries": 2,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d; want 201", resp.StatusCode)
	}
	data = decodeBody(t, resp)
	if data["total_entries"] != float64(2) {
		t.Errorf("total_entries = %v; want 2", data["total_entries"])
	}
	if data["total_cost"] != float64(200) {
		t.Errorf("total_cost = %v; want 200", data["total_cost"])
	}
	if data["signup_instructions"] != "Pay the gil!" {
		t.Errorf("signup_instructions = %v", data["signup_instructions"])
	}
}

func TestRaffles_EnterAddsToExisting(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Add Test", "max_entries": 10, "cost_per_entry": 50,
	})
	data := decodeBody(t, resp)
	id := int(data["raffle"].(map[string]any)["id"].(float64))

	// First entry
	env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", id), map[string]any{
		"character_name": "Player", "world": "World", "num_entries": 3,
	}).Body.Close()

	// Second entry for same player — should add
	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", id), map[string]any{
		"character_name": "Player", "world": "World", "num_entries": 2,
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d; want 200", resp.StatusCode)
	}
	data = decodeBody(t, resp)
	if data["total_entries"] != float64(5) {
		t.Errorf("total_entries = %v; want 5", data["total_entries"])
	}
}

func TestRaffles_EnterExceedsMax(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Max Test", "max_entries": 3,
	})
	data := decodeBody(t, resp)
	id := int(data["raffle"].(map[string]any)["id"].(float64))

	// Try to enter more than max
	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", id), map[string]any{
		"character_name": "Player", "world": "World", "num_entries": 5,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRaffles_EnterMissingFields(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Field Test", "max_entries": 5,
	})
	data := decodeBody(t, resp)
	id := int(data["raffle"].(map[string]any)["id"].(float64))

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", id), map[string]any{
		"character_name": "", "world": "", "num_entries": 1,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRaffles_EnterClosedRaffle(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Closed Test", "max_entries": 5,
	})
	data := decodeBody(t, resp)
	id := int(data["raffle"].(map[string]any)["id"].(float64))

	// Close the raffle via store directly
	_ = env.store.SetRaffleStatus(int64(id), "closed")

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", id), map[string]any{
		"character_name": "Player", "world": "World", "num_entries": 1,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRaffles_EntriesRequiresAuth(t *testing.T) {
	env := newTestEnv(t)

	resp := env.postJSON(t, "/api/raffles/1/entries", map[string]any{
		"action": "mark_paid", "entry_id": 1, "paid": true,
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRaffles_MarkPaid(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Create raffle and entry
	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Paid Test", "max_entries": 5,
	})
	data := decodeBody(t, resp)
	raffleID := int(data["raffle"].(map[string]any)["id"].(float64))

	entryID, _ := env.store.CreateRaffleEntry(int64(raffleID), "P1", "W1", 1)

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"action": "mark_paid", "entry_id": entryID, "paid": true,
	})
	data = decodeBody(t, resp)
	if data["ok"] != true {
		t.Errorf("expected ok=true, got %v", data["ok"])
	}

	// Verify paid
	entry, _ := env.store.GetRaffleEntry(int64(raffleID), "P1", "W1")
	if !entry.Paid {
		t.Error("expected entry to be paid")
	}
}

func TestRaffles_PickWinner(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Winner Test", "max_entries": 5,
	})
	data := decodeBody(t, resp)
	raffleID := int(data["raffle"].(map[string]any)["id"].(float64))

	entryID, _ := env.store.CreateRaffleEntry(int64(raffleID), "Winner", "World", 3)
	_ = env.store.SetRaffleEntryPaid(entryID, true)

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"action": "pick_winner",
	})
	data = decodeBody(t, resp)
	winner, _ := data["winner"].(map[string]any)
	if winner == nil {
		t.Fatal("expected winner in response")
	}
	if winner["character_name"] != "Winner" {
		t.Errorf("winner = %v; want Winner", winner["character_name"])
	}
}

func TestRaffles_PickWinnerNoPaid(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "No Paid Test", "max_entries": 5,
	})
	data := decodeBody(t, resp)
	raffleID := int(data["raffle"].(map[string]any)["id"].(float64))

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"action": "pick_winner",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRaffles_VerifyWinner(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Verify Test", "max_entries": 5,
	})
	data := decodeBody(t, resp)
	raffleID := int(data["raffle"].(map[string]any)["id"].(float64))

	entryID, _ := env.store.CreateRaffleEntry(int64(raffleID), "W", "X", 1)
	_ = env.store.SetRaffleEntryPaid(entryID, true)
	_ = env.store.SetRaffleWinner(int64(raffleID), &entryID)

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"action": "verify_winner",
	})
	data = decodeBody(t, resp)
	if data["status"] != "closed" {
		t.Errorf("status = %v; want closed", data["status"])
	}

	// Verify raffle is actually closed
	r, _ := env.store.GetRaffle(int64(raffleID))
	if r.Status != "closed" {
		t.Errorf("raffle status = %q; want closed", r.Status)
	}
}

func TestRaffles_VerifyWinnerNoWinner(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "No Winner Test", "max_entries": 5,
	})
	data := decodeBody(t, resp)
	raffleID := int(data["raffle"].(map[string]any)["id"].(float64))

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"action": "verify_winner",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRaffles_DeleteEntry(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Del Entry Test", "max_entries": 5,
	})
	data := decodeBody(t, resp)
	raffleID := int(data["raffle"].(map[string]any)["id"].(float64))

	entryID, _ := env.store.CreateRaffleEntry(int64(raffleID), "P", "W", 1)

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"action": "delete_entry", "entry_id": entryID,
	})
	data = decodeBody(t, resp)
	if data["deleted"] != true {
		t.Errorf("expected deleted=true, got %v", data["deleted"])
	}
}

func TestRaffles_EntriesInvalidAction(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "T", "max_entries": 1,
	})
	data := decodeBody(t, resp)
	raffleID := int(data["raffle"].(map[string]any)["id"].(float64))

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"action": "explode",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRaffles_AddEntry(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Add Entry Test", "max_entries": 5, "cost_per_entry": 100,
	})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"action": "add_entry", "character_name": "Cloud", "world": "Gaia", "num_entries": 2,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	entry, _ := decodeBody(t, resp)["entry"].(map[string]any)
	if entry == nil {
		t.Fatal("expected entry in response")
	}
	if entry["character_name"] != "Cloud" || entry["num_entries"] != float64(2) {
		t.Errorf("entry = %v; want Cloud/2", entry)
	}
	if entry["paid"] != false {
		t.Errorf("paid = %v; want false (not requested)", entry["paid"])
	}

	// Confirm it persisted, unpaid.
	stored, _ := env.store.GetRaffleEntry(int64(raffleID), "Cloud", "Gaia")
	if stored == nil || stored.NumEntries != 2 || stored.Paid {
		t.Errorf("stored = %+v; want 2 entries, unpaid", stored)
	}
}

func TestRaffles_AddEntryPaid(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Add Paid Test", "max_entries": 5,
	})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"action": "add_entry", "character_name": "Tifa", "world": "Gaia", "num_entries": 1, "paid": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	stored, _ := env.store.GetRaffleEntry(int64(raffleID), "Tifa", "Gaia")
	if stored == nil || !stored.Paid {
		t.Errorf("expected stored entry to be paid, got %+v", stored)
	}
}

func TestRaffles_AddEntryMergesExisting(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Merge Test", "max_entries": 10,
	})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))

	// Pre-existing paid entry.
	entryID, _ := env.store.CreateRaffleEntry(int64(raffleID), "Aerith", "Gaia", 3)
	_ = env.store.SetRaffleEntryPaid(entryID, true)

	// Admin adds 2 more (case-insensitive match) WITHOUT the paid flag: the
	// tickets must merge and the existing paid status must be preserved.
	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"action": "add_entry", "character_name": "aerith", "world": "GAIA", "num_entries": 2,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	stored, _ := env.store.GetRaffleEntry(int64(raffleID), "Aerith", "Gaia")
	if stored == nil || stored.NumEntries != 5 {
		t.Errorf("num_entries = %+v; want 5 (3+2 merged)", stored)
	}
	if stored != nil && !stored.Paid {
		t.Error("expected existing paid status to be preserved")
	}
}

func TestRaffles_AddEntryExceedsMax(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Max Test", "max_entries": 3,
	})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"action": "add_entry", "character_name": "P", "world": "W", "num_entries": 5,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRaffles_AddEntryMissingFields(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Field Test", "max_entries": 5,
	})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"action": "add_entry", "character_name": "", "world": "", "num_entries": 1,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRaffles_AddEntryClosedRaffle(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Closed Test", "max_entries": 5,
	})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))
	_ = env.store.SetRaffleStatus(int64(raffleID), "closed")

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"action": "add_entry", "character_name": "P", "world": "W", "num_entries": 1,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRaffles_AddEntryIgnoresAvailabilityWindow(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Availability window already closed, but the raffle itself is still open.
	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Window Test", "max_entries": 5,
		"available_to": "2000-01-01T00:00",
	})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))

	// Public sign-up is rejected (past the window)…
	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", raffleID), map[string]any{
		"character_name": "P", "world": "W", "num_entries": 1,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("public enter status = %d; want 400 (window closed)", resp.StatusCode)
	}
	resp.Body.Close()

	// …but the admin can still add manually.
	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"action": "add_entry", "character_name": "P", "world": "W", "num_entries": 1,
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("admin add status = %d; want 200 (window ignored)", resp.StatusCode)
	}
	resp.Body.Close()
}

// ── Styles ──────────────────────────────────────────────────────────────────

func TestStyles_RequiresAuth(t *testing.T) {
	env := newTestEnv(t)

	resp := env.postJSON(t, "/api/styles", map[string]any{"action": "create", "name": "T", "css": "x"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestStyles_ActiveNoStyle(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/styles/active")
	data := decodeBody(t, resp)
	if data["css"] != "" {
		t.Errorf("expected empty CSS, got %v", data["css"])
	}
}

// ── Winners Log ─────────────────────────────────────────────────────────────

func TestWinnersLog_RequiresAuth(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/winners-log")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestWinnersLog_Empty(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.get(t, "/api/winners-log")
	data := decodeBody(t, resp)
	entries, _ := data["entries"].([]any)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
	if data["total"] != float64(0) {
		t.Errorf("total = %v; want 0", data["total"])
	}
}

// ── Settings ────────────────────────────────────────────────────────────────

func TestSettings_GetDefaults(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/settings")
	data := decodeBody(t, resp)
	settings, _ := data["settings"].(map[string]any)
	if settings == nil {
		t.Fatal("expected settings map")
	}
	if settings["app_title"] != "Nifty App Suite" {
		t.Errorf("app_title = %v; want default", settings["app_title"])
	}
	if settings["default_draw_delay"] != "0" {
		t.Errorf("default_draw_delay = %v; want 0", settings["default_draw_delay"])
	}
	if settings["frequent_winner_threshold"] != "3" {
		t.Errorf("frequent_winner_threshold = %v; want 3", settings["frequent_winner_threshold"])
	}
	if settings["frequent_winner_hours"] != "12" {
		t.Errorf("frequent_winner_hours = %v; want 12", settings["frequent_winner_hours"])
	}
	if settings["header_font"] != "Arapey" {
		t.Errorf("header_font = %v; want Arapey", settings["header_font"])
	}
}

func TestSettings_UpdateRequiresAuth(t *testing.T) {
	env := newTestEnv(t)

	resp := env.postJSON(t, "/api/settings", map[string]any{
		"settings": map[string]string{"app_title": "New Title"},
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSettings_Update(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/settings", map[string]any{
		"settings": map[string]string{
			"app_title":          "My Custom App",
			"default_draw_delay": "10",
		},
	})
	data := decodeBody(t, resp)
	if data["ok"] != true {
		t.Errorf("expected ok=true, got %v", data["ok"])
	}

	// Verify settings persisted
	resp = env.get(t, "/api/settings")
	data = decodeBody(t, resp)
	settings, _ := data["settings"].(map[string]any)
	if settings["app_title"] != "My Custom App" {
		t.Errorf("app_title = %v; want My Custom App", settings["app_title"])
	}
	if settings["default_draw_delay"] != "10" {
		t.Errorf("default_draw_delay = %v; want 10", settings["default_draw_delay"])
	}
}

func TestSettings_InvalidKey(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/settings", map[string]any{
		"settings": map[string]string{"bad_key": "value"},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSettings_InvalidDrawDelay(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/settings", map[string]any{
		"settings": map[string]string{"default_draw_delay": "999"},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSettings_InvalidThreshold(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/settings", map[string]any{
		"settings": map[string]string{"frequent_winner_threshold": "0"},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}
