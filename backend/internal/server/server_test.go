package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"app-suite/internal/api/apitest"
	"app-suite/internal/live"
	"app-suite/internal/model"
	"app-suite/internal/server"
	"app-suite/internal/store"
	"app-suite/internal/store/storetest"
)

// createRaffleEntry adds a raffle entry for test setup via the production
// cap-enforcing writer (with an effectively unlimited cap) and returns its id.
func createRaffleEntry(t *testing.T, s *store.Store, raffleID int64, name, world string, num int) int64 {
	t.Helper()
	id, _, _, _, err := s.AddOrCreateRaffleEntry(t.Context(), raffleID, name, world, num, 1_000_000)
	if err != nil {
		t.Fatalf("create raffle entry: %v", err)
	}
	return id
}

// getRaffleEntryByName looks up a raffle entry by character+world (case-insensitive)
// via the production list query, returning nil when none matches.
func getRaffleEntryByName(t *testing.T, s *store.Store, raffleID int64, name, world string) *model.RaffleEntry {
	t.Helper()
	entries, err := s.ListRaffleEntries(t.Context(), raffleID)
	if err != nil {
		t.Fatalf("list raffle entries: %v", err)
	}
	for i := range entries {
		if strings.EqualFold(entries[i].CharacterName, name) && strings.EqualFold(entries[i].World, world) {
			return &entries[i]
		}
	}
	return nil
}

const (
	// Bootstrap account seeded by the users migration (see store.migrateUsers).
	seedAdminUser = "admin"
	seedAdminPass = "admin"
)

// testEnv bundles a test server, authenticated client, and raw store.
type testEnv struct {
	ts     *httptest.Server
	client *http.Client
	store  *store.Store
	srv    *server.Server
	hub    *live.Hub
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	st, err := store.New(t.Context(), storetest.Path(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	hub := live.New()
	srv := server.New(t.Context(), st, hub, t.TempDir(), nil)

	ts := httptest.NewTLSServer(srv)
	// Cleanups run last first: the hub closes first, ending any stream still open,
	// so the server's close doesn't wait on one.
	t.Cleanup(ts.Close)
	t.Cleanup(hub.Close)

	jar, _ := cookiejar.New(nil)
	client := ts.Client()
	client.Jar = jar
	client.Transport = apitest.Transport(t, client.Transport)

	return &testEnv{ts: ts, client: client, store: st, srv: srv, hub: hub}
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

// reqJSON sends an arbitrary-method JSON request (PUT/PATCH/DELETE) for the
// resource-oriented routes. A nil body sends no body (for DELETE).
func (e *testEnv) reqJSON(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, e.url(path), r)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func (e *testEnv) putJSON(t *testing.T, path string, body any) *http.Response {
	return e.reqJSON(t, http.MethodPut, path, body)
}
func (e *testEnv) patchJSON(t *testing.T, path string, body any) *http.Response {
	return e.reqJSON(t, http.MethodPatch, path, body)
}
func (e *testEnv) del(t *testing.T, path string) *http.Response {
	return e.reqJSON(t, http.MethodDelete, path, nil)
}

func (e *testEnv) loginAdmin(t *testing.T) {
	t.Helper()
	resp := e.postJSON(t, "/api/v1/auth/login", map[string]string{
		"username": seedAdminUser, "password": seedAdminPass,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin login failed: %d", resp.StatusCode)
	}
}

// signOut ends the client's session.
func (e *testEnv) signOut(t *testing.T) {
	t.Helper()
	resp := e.reqJSON(t, http.MethodPost, "/api/v1/auth/logout", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("sign-out: %d", resp.StatusCode)
	}
}

// signedInAs is who GET /api/v1/me says the client is: a username, or "" for nobody.
func (e *testEnv) signedInAs(t *testing.T) string {
	t.Helper()
	return accountName(t, e.get(t, "/api/v1/me"))
}

// accountName is the username a who-am-I answer names, or "" for nobody.
func accountName(t *testing.T, resp *http.Response) string {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("who am I: %d", resp.StatusCode)
	}
	account, _ := decodeBody(t, resp)["account"].(map[string]any)
	name, _ := account["username"].(string)
	return name
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

// -- CSRF --------------------------------------------------------------------

// TestCSRF_CrossOriginMutationBlocked verifies the defense-in-depth Origin check:
// a state-changing request carrying the session cookie but a cross-origin Origin
// header is rejected 403, while a same-origin Origin and an absent Origin pass
// through to the handler.
func TestCSRF_CrossOriginMutationBlocked(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	post := func(origin string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, env.url("/api/v1/styles/deactivate"),
			bytes.NewReader([]byte(`{}`)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := env.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	resp := post("https://evil.example")
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin POST status = %d; want 403", resp.StatusCode)
	}

	resp = post(env.ts.URL) // same origin as the test server
	resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		t.Errorf("same-origin POST was blocked (403); want it allowed through")
	}

	resp = post("") // no Origin header - SameSite cookie stays the primary defense
	resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		t.Errorf("Origin-less POST was blocked (403); want it allowed through")
	}
}

// -- CORS --------------------------------------------------------------------

// TestCORS_OptionsPreflight verifies OPTIONS is short-circuited with 204 and,
// under the default (empty) allowlist, carries no CORS headers.
func TestCORS_OptionsPreflight(t *testing.T) {
	env := newTestEnv(t)

	req, _ := http.NewRequest(http.MethodOptions, env.url("/api/v1/me"), nil)
	req.Header.Set("Origin", "http://evil.example")
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d; want 204", resp.StatusCode)
	}
	if h := resp.Header.Get("Access-Control-Allow-Origin"); h != "" {
		t.Errorf("Access-Control-Allow-Origin = %q; want empty for an unlisted origin", h)
	}
}

// TestCORS_UnlistedOriginGetsNoHeaders locks in the secure default: an origin
// not on the allowlist gets no CORS headers, so no arbitrary site can read
// credentialed responses. (The allowlist is normally empty - same-origin app.)
func TestCORS_UnlistedOriginGetsNoHeaders(t *testing.T) {
	env := newTestEnv(t)

	req, _ := http.NewRequest(http.MethodGet, env.url("/api/v1/game"), nil)
	req.Header.Set("Origin", "http://evil.example")
	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if h := resp.Header.Get("Access-Control-Allow-Origin"); h != "" {
		t.Errorf("Access-Control-Allow-Origin = %q; want empty (origin not allowlisted)", h)
	}
}

// TestCORS_AllowlistedOriginReflected verifies an explicitly allow-listed origin
// is echoed back with credentials enabled.
func TestCORS_AllowlistedOriginReflected(t *testing.T) {
	st, err := store.New(t.Context(), storetest.Path(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	const origin = "https://app.example"
	srv := server.New(t.Context(), st, live.New(), t.TempDir(), []string{origin})
	ts := httptest.NewTLSServer(srv)
	t.Cleanup(ts.Close)

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/game", nil)
	req.Header.Set("Origin", origin)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if h := resp.Header.Get("Access-Control-Allow-Origin"); h != origin {
		t.Errorf("Access-Control-Allow-Origin = %q; want %q", h, origin)
	}
	if h := resp.Header.Get("Access-Control-Allow-Credentials"); h != "true" {
		t.Errorf("Access-Control-Allow-Credentials = %q; want true", h)
	}
}

// -- Auth --------------------------------------------------------------------

func TestAuth_CheckNotAuthenticated(t *testing.T) {
	env := newTestEnv(t)

	data := decodeBody(t, env.get(t, "/api/v1/me"))
	if account, present := data["account"]; !present || account != nil {
		t.Errorf("signed out, account = %v (present %v); want null", account, present)
	}
}

func TestAuth_LoginSuccess(t *testing.T) {
	env := newTestEnv(t)

	resp := env.postJSON(t, "/api/v1/auth/login", map[string]string{
		"username": seedAdminUser, "password": seedAdminPass,
	})
	if name := accountName(t, resp); name != seedAdminUser {
		t.Errorf("sign-in answered %q; want %q", name, seedAdminUser)
	}
	if name := env.signedInAs(t); name != seedAdminUser {
		t.Errorf("after signing in, who am I = %q; want %q", name, seedAdminUser)
	}
}

func TestAuth_LoginWrongPassword(t *testing.T) {
	env := newTestEnv(t)

	resp := env.postJSON(t, "/api/v1/auth/login", map[string]string{
		"username": seedAdminUser, "password": "wrong",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAuth_Logout(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	env.signOut(t)

	if name := env.signedInAs(t); name != "" {
		t.Errorf("after signing out, who am I = %q; want nobody", name)
	}
}

// -- JSON error format (the routes not yet on huma) ---------------------------

func TestErrorFormat(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/raffles/99999")
	data := decodeBody(t, resp)

	if _, ok := data["error"]; !ok {
		t.Error("expected error key in JSON error response")
	}
}

func TestJSONContentType(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/raffles")
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q; want application/json; charset=utf-8", ct)
	}
}

func TestCacheControlNoStore(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/raffles")
	defer resp.Body.Close()

	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q; want no-store", cc)
	}
}

// -- Raffles -----------------------------------------------------------------

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

	resp = env.putJSON(t, fmt.Sprintf("/api/raffles/%d", int(id)), map[string]any{
		"title": "Updated", "max_entries": 10,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	data = decodeBody(t, resp)
	raffle = data["raffle"].(map[string]any)
	if raffle["title"] != "Updated" {
		t.Errorf("title = %v; want Updated", raffle["title"])
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

	resp = env.del(t, fmt.Sprintf("/api/raffles/%d", int(id)))
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()
	// Confirm it's gone.
	if r, _ := env.store.GetRaffle(t.Context(), int64(id)); r != nil {
		t.Error("raffle should be deleted")
	}
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

// TestRaffles_CreateCustomCost covers the "Custom Cost per Entry" mode: the
// ladder is stored as sent, and max_entries is pinned to its length regardless
// of what the client asked for (a 4th ticket in a 3-rung ladder has no price).
func TestRaffles_CreateCustomCost(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Tiered", "entry_mode": "custom",
		"max_entries": 99, "cost_per_entry": 777,
		"tier_costs": []any{50000, 100000, 150000},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d; want 201", resp.StatusCode)
	}
	raffle := decodeBody(t, resp)["raffle"].(map[string]any)
	if raffle["entry_mode"] != "custom" {
		t.Errorf("entry_mode = %v; want custom", raffle["entry_mode"])
	}
	if raffle["max_entries"] != float64(3) {
		t.Errorf("max_entries = %v; want 3 (the ladder length)", raffle["max_entries"])
	}
	// The flat cost belongs to the other mode and must not linger as a shadow price.
	if raffle["cost_per_entry"] != float64(0) {
		t.Errorf("cost_per_entry = %v; want 0 in custom mode", raffle["cost_per_entry"])
	}
	tiers, _ := raffle["tier_costs"].([]any)
	if len(tiers) != 3 || tiers[2] != float64(150000) {
		t.Errorf("tier_costs = %v; want [50000 100000 150000]", raffle["tier_costs"])
	}
}

// TestRaffles_CreateCustomCostNeedsTiers rejects a custom raffle with no ladder -
// there would be no price for any ticket.
func TestRaffles_CreateCustomCostNeedsTiers(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Tiered", "entry_mode": "custom", "tier_costs": []any{},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	resp = env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Tiered", "entry_mode": "custom", "tier_costs": []any{50000, -1},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("negative tier: status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestRaffles_EnterCustomCost checks the sign-up total climbs the ladder rather
// than multiplying one price: 3 tickets at 50k/100k/150k owe 300,000.
func TestRaffles_EnterCustomCost(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Tiered", "entry_mode": "custom",
		"tier_costs": []any{50000, 100000, 150000},
	})
	id := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", id), map[string]any{
		"character_name": "Aria", "world": "Gilgamesh", "num_entries": 3,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d; want 201", resp.StatusCode)
	}
	data := decodeBody(t, resp)
	if data["total_cost"] != float64(300000) {
		t.Errorf("total_cost = %v; want 300000", data["total_cost"])
	}

	// A 4th ticket is over the ladder-derived cap.
	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", id), map[string]any{
		"character_name": "Aria", "world": "Gilgamesh", "num_entries": 1,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("over-cap entry: status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestRaffles_EnterDetailsOnlyRejected covers the "Details Only" mode: the raffle
// is published for reference and entered elsewhere, so the public entry endpoint
// refuses it even though the raffle is open and in-window.
func TestRaffles_EnterDetailsOnlyRejected(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Info Only", "entry_mode": "details", "max_entries": 3,
		"signup_instructions": "Whisper a staff member in game.",
	})
	id := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", id), map[string]any{
		"character_name": "Aria", "world": "Gilgamesh", "num_entries": 1,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestRaffles_PickWinnerDetailsOnly checks a details-only raffle draws from every
// entry: staff record them from a sign-up run elsewhere, so none is marked paid
// and a paid-only draw would always come up empty.
func TestRaffles_PickWinnerDetailsOnly(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Info Only", "entry_mode": "details", "max_entries": 3,
	})
	id := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))

	env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", id), map[string]any{
		"character_name": "Aria", "world": "Gilgamesh", "num_entries": 1, "paid": false,
	}).Body.Close()

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/pick-winner", id), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	winner := decodeBody(t, resp)["winner"].(map[string]any)
	if winner["character_name"] != "Aria" {
		t.Errorf("winner = %v; want Aria", winner["character_name"])
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

	// Second entry for same player - should add
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

// TestRaffles_EnterRejectsOverflowingCount pins the ceiling on num_entries. The
// handler used to floor the value at 1 and never cap it, so an unauthenticated
// sign-up could post a count near MaxInt64: the store's cap check summed it with
// the existing total, wrapped negative, passed the cap, and wrote a num_entries
// SQLite had to widen to REAL - permanently 500ing the staff raffle view and the
// entry-delete route for that raffle. Two sign-ups are needed to reach the
// overflow, so the second is the one that must be refused.
func TestRaffles_EnterRejectsOverflowingCount(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Overflow Test", "max_entries": 3,
	})
	data := decodeBody(t, resp)
	id := int(data["raffle"].(map[string]any)["id"].(float64))

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", id), map[string]any{
		"character_name": "Aria", "world": "Gilgamesh", "num_entries": 1,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first entry status = %d; want 201", resp.StatusCode)
	}
	resp.Body.Close()

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", id), map[string]any{
		"character_name": "Aria", "world": "Gilgamesh", "num_entries": math.MaxInt64,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("overflowing entry status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// The raffle must still be readable. This is the assertion that actually
	// catches the regression: an overflowed count breaks the entry scan, so the
	// detail view 500s rather than merely reporting a wrong total.
	resp = env.get(t, fmt.Sprintf("/api/raffles/%d", id))
	if resp.StatusCode != http.StatusOK {
		t.Errorf("raffle detail after overflowing entry: status = %d; want 200", resp.StatusCode)
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
	_ = env.store.SetRaffleStatus(t.Context(), int64(id), "closed")

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

	entryID := createRaffleEntry(t, env.store, int64(raffleID), "P1", "W1", 1)

	resp = env.patchJSON(t, fmt.Sprintf("/api/raffles/%d/entries/%d", raffleID, entryID), map[string]any{
		"paid": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	entry, _ := decodeBody(t, resp)["entry"].(map[string]any)
	if entry == nil || entry["paid"] != true {
		t.Errorf("expected paid entry in response, got %v", entry)
	}

	// Verify paid
	stored := getRaffleEntryByName(t, env.store, int64(raffleID), "P1", "W1")
	if !stored.Paid {
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

	entryID := createRaffleEntry(t, env.store, int64(raffleID), "Winner", "World", 3)
	_, _ = env.store.SetRaffleEntryPaid(t.Context(), entryID, true, 0, 0)

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/pick-winner", raffleID), nil)
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

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/pick-winner", raffleID), nil)
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

	entryID := createRaffleEntry(t, env.store, int64(raffleID), "W", "X", 1)
	_, _ = env.store.SetRaffleEntryPaid(t.Context(), entryID, true, 0, 0)
	_ = env.store.SetRaffleWinner(t.Context(), int64(raffleID), &entryID)

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/verify-winner", raffleID), nil)
	data = decodeBody(t, resp)
	if data["status"] != "closed" {
		t.Errorf("status = %v; want closed", data["status"])
	}

	// Verify raffle is actually closed
	r, _ := env.store.GetRaffle(t.Context(), int64(raffleID))
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

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/verify-winner", raffleID), nil)
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

	entryID := createRaffleEntry(t, env.store, int64(raffleID), "P", "W", 1)

	resp = env.del(t, fmt.Sprintf("/api/raffles/%d/entries/%d", raffleID, entryID))
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d; want 204", resp.StatusCode)
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
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d; want 201", resp.StatusCode)
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
	stored := getRaffleEntryByName(t, env.store, int64(raffleID), "Cloud", "Gaia")
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
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d; want 201", resp.StatusCode)
	}
	resp.Body.Close()

	stored := getRaffleEntryByName(t, env.store, int64(raffleID), "Tifa", "Gaia")
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

	// Pre-existing settled entry, with one of its tickets waived.
	entryID := createRaffleEntry(t, env.store, int64(raffleID), "Aerith", "Gaia", 3)
	_, _ = env.store.SetRaffleEntryPaid(t.Context(), entryID, true, 0, 500)

	// Admin adds 2 more (case-insensitive match) WITHOUT the paid flag: the
	// tickets merge onto the same row, and because those two are NOT settled the
	// row drops out of "paid" into partial - what it already collected and waived
	// stays on record.
	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"action": "add_entry", "character_name": "aerith", "world": "GAIA", "num_entries": 2,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	stored := getRaffleEntryByName(t, env.store, int64(raffleID), "Aerith", "Gaia")
	if stored == nil || stored.NumEntries != 5 {
		t.Fatalf("num_entries = %+v; want 5 (3+2 merged)", stored)
	}
	if stored.Paid {
		t.Error("expected the row to fall out of paid once unsettled tickets were added")
	}
	if stored.PaidEntries != 3 {
		t.Errorf("paid_entries = %d; want 3 (what was actually settled)", stored.PaidEntries)
	}
	if stored.AmountWaived != 500 {
		t.Errorf("amount_waived = %v; want 500 (preserved across the merge)", stored.AmountWaived)
	}
	if stored.PaymentState() != "partial" {
		t.Errorf("state = %q; want partial", stored.PaymentState())
	}
}

// TestRaffles_MarkPaidWaiverAccumulates drives the waiver through the HTTP
// surface: two settlements on the same entry add up rather than overwriting, and
// clearing the settlement wipes both counters.
func TestRaffles_MarkPaidWaiverAccumulates(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Waiver", "max_entries": 3, "cost_per_entry": 50000,
	})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))
	entryID := createRaffleEntry(t, env.store, int64(raffleID), "Aria", "Gilgamesh", 1)

	path := fmt.Sprintf("/api/raffles/%d/entries/%d", raffleID, entryID)
	entry := decodeBody(t, env.patchJSON(t, path, map[string]any{"paid": true, "amount_waived": 10000}))["entry"].(map[string]any)
	if entry["amount_waived"] != float64(10000) || entry["paid_entries"] != float64(1) {
		t.Fatalf("first settlement = %+v; want amount_waived=10000 paid_entries=1", entry)
	}

	// Two more tickets, then settle again with another waiver.
	env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"character_name": "Aria", "world": "Gilgamesh", "num_entries": 2,
	}).Body.Close()
	entry = decodeBody(t, env.patchJSON(t, path, map[string]any{"paid": true, "amount_waived": 5000}))["entry"].(map[string]any)
	if entry["amount_waived"] != float64(15000) {
		t.Errorf("amount_waived = %v; want 15000 (10000 + 5000)", entry["amount_waived"])
	}
	if entry["paid_entries"] != float64(3) || entry["paid"] != true {
		t.Errorf("second settlement = %+v; want paid_entries=3 paid=true", entry)
	}

	// Clearing resets the row - no waiver left credited against an unpaid entry.
	entry = decodeBody(t, env.patchJSON(t, path, map[string]any{"paid": false}))["entry"].(map[string]any)
	if entry["amount_waived"] != float64(0) || entry["paid_entries"] != float64(0) || entry["paid"] != false {
		t.Errorf("after clearing = %+v; want everything reset", entry)
	}
}

// TestRaffles_MarkPaidRejectsBadWaiver keeps a negative or non-finite waiver out
// of the row - either would corrupt every collected-gil total that reads it.
// TestRaffles_MarkPaidPartial records a part payment over HTTP - gil for two of
// three entries now, the rest later - and checks the entry reads as partial
// rather than paid.
func TestRaffles_MarkPaidPartial(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Part", "max_entries": 3, "cost_per_entry": 50000,
	})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))
	entryID := createRaffleEntry(t, env.store, int64(raffleID), "Aria", "Gilgamesh", 3)
	path := fmt.Sprintf("/api/raffles/%d/entries/%d", raffleID, entryID)

	entry := decodeBody(t, env.patchJSON(t, path, map[string]any{
		"paid": true, "paid_entries": 2, "amount_waived": 10000,
	}))["entry"].(map[string]any)
	if entry["paid_entries"] != float64(2) {
		t.Errorf("paid_entries = %v; want 2", entry["paid_entries"])
	}
	if entry["paid"] != false {
		t.Error("an entry with a ticket still owing must not read as paid")
	}
	if entry["amount_waived"] != float64(10000) {
		t.Errorf("amount_waived = %v; want 10000", entry["amount_waived"])
	}

	// Omitting the count settles everything, which is the common counter case.
	entry = decodeBody(t, env.patchJSON(t, path, map[string]any{"paid": true}))["entry"].(map[string]any)
	if entry["paid_entries"] != float64(3) || entry["paid"] != true {
		t.Errorf("entry = %+v; want all 3 settled and paid", entry)
	}

	// A negative count is rejected rather than quietly floored.
	resp = env.patchJSON(t, path, map[string]any{"paid": true, "paid_entries": -1})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("negative paid_entries = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRaffles_MarkPaidRejectsBadWaiver(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Waiver", "max_entries": 1, "cost_per_entry": 50000,
	})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))
	entryID := createRaffleEntry(t, env.store, int64(raffleID), "Aria", "Gilgamesh", 1)

	resp = env.patchJSON(t, fmt.Sprintf("/api/raffles/%d/entries/%d", raffleID, entryID),
		map[string]any{"paid": true, "amount_waived": -1})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestRaffles_WinnerEntryHidesSettlement keeps the settlement columns staff-only.
// The winner of a closed raffle is shown publicly by name, so leaking that they
// had their entries comped would put a private arrangement on the public page.
func TestRaffles_WinnerEntryHidesSettlement(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Closed", "max_entries": 3, "cost_per_entry": 50000,
	})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))
	entryID := createRaffleEntry(t, env.store, int64(raffleID), "Aria", "Gilgamesh", 3)
	env.patchJSON(t, fmt.Sprintf("/api/raffles/%d/entries/%d", raffleID, entryID),
		map[string]any{"paid": true, "amount_waived": 150000}).Body.Close()
	env.postJSON(t, fmt.Sprintf("/api/raffles/%d/pick-winner", raffleID), nil).Body.Close()
	env.postJSON(t, fmt.Sprintf("/api/raffles/%d/verify-winner", raffleID), nil).Body.Close()

	// Staff still see the whole settlement.
	entries := decodeBody(t, env.get(t, fmt.Sprintf("/api/raffles/%d", raffleID)))["entries"].([]any)
	staffEntry := entries[0].(map[string]any)
	if staffEntry["amount_waived"] != float64(150000) {
		t.Fatalf("staff amount_waived = %v; want 150000", staffEntry["amount_waived"])
	}

	env.signOut(t)
	winner := decodeBody(t, env.get(t, fmt.Sprintf("/api/raffles/%d", raffleID)))["winner_entry"].(map[string]any)
	if winner["character_name"] != "Aria" {
		t.Fatalf("winner_entry = %+v; want the winner shown", winner)
	}
	if winner["amount_waived"] != float64(0) {
		t.Errorf("public amount_waived = %v; want 0 (staff-only)", winner["amount_waived"])
	}
	if winner["paid_entries"] != float64(0) {
		t.Errorf("public paid_entries = %v; want 0 (staff-only)", winner["paid_entries"])
	}
}

// TestRaffles_EnterAmountDueSkipsSettledTickets is the returning-entrant case:
// somebody who already paid for a ticket and comes back for two more must be
// quoted for the NEW ones only. Quoting the full ladder would invite them to pay
// for the first ticket twice.
func TestRaffles_EnterAmountDueSkipsSettledTickets(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Tiered", "entry_mode": "custom",
		"tier_costs": []any{50000, 100000, 150000},
	})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))

	// First entry: nothing settled, so the two figures agree.
	body := decodeBody(t, env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", raffleID), map[string]any{
		"character_name": "Aria", "world": "Gilgamesh", "num_entries": 1,
	}))
	if body["total_cost"] != float64(50000) || body["amount_due"] != float64(50000) {
		t.Fatalf("first sign-up = %+v; want both 50000", body)
	}

	// Staff settle that ticket, waiving half of it.
	entryID := getRaffleEntryByName(t, env.store, int64(raffleID), "Aria", "Gilgamesh").ID
	env.patchJSON(t, fmt.Sprintf("/api/raffles/%d/entries/%d", raffleID, entryID),
		map[string]any{"paid": true, "amount_waived": 25000}).Body.Close()

	// Coming back for two more: the sticker price is the whole ladder, but only the
	// two new tickets are owed.
	body = decodeBody(t, env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", raffleID), map[string]any{
		"character_name": "Aria", "world": "Gilgamesh", "num_entries": 2,
	}))
	if body["total_cost"] != float64(300000) {
		t.Errorf("total_cost = %v; want 300000 (all three tickets)", body["total_cost"])
	}
	if body["amount_due"] != float64(250000) {
		t.Errorf("amount_due = %v; want 250000 (the two new tickets only)", body["amount_due"])
	}
}

// TestRaffles_MaxEntriesCapped rejects an allowance past the cap rather than
// silently storing a different one, on create and on update, and for the custom
// ladder whose length IS the allowance.
func TestRaffles_MaxEntriesCapped(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Too many", "max_entries": 101,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("create over cap: status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// The cap itself is allowed.
	resp = env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "At the cap", "max_entries": 100,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create at cap: status = %d; want 201", resp.StatusCode)
	}
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))

	resp = env.putJSON(t, fmt.Sprintf("/api/raffles/%d", raffleID), map[string]any{
		"title": "At the cap", "max_entries": 101,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("update over cap: status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	ladder := make([]any, 101)
	for i := range ladder {
		ladder[i] = 100
	}
	resp = env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Long ladder", "entry_mode": "custom", "tier_costs": ladder,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("ladder over cap: status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestRaffles_EntryLookup drives the public search over HTTP: it finds a partial
// name, refuses a one-character query, and never carries money out with it.
func TestRaffles_EntryLookup(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Search", "max_entries": 3, "cost_per_entry": 50000,
	})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))
	entryID := createRaffleEntry(t, env.store, int64(raffleID), "Aria Fairwind", "Gilgamesh", 3)
	env.patchJSON(t, fmt.Sprintf("/api/raffles/%d/entries/%d", raffleID, entryID),
		map[string]any{"paid": true, "amount_waived": 90000}).Body.Close()
	env.signOut(t)

	path := fmt.Sprintf("/api/raffles/%d/lookup", raffleID)
	body := decodeBody(t, env.postJSON(t, path, map[string]any{"name": "fairwind"}))
	hits := body["entries"].([]any)
	if len(hits) != 1 {
		t.Fatalf("entries = %+v; want one hit", hits)
	}
	hit := hits[0].(map[string]any)
	if hit["character_name"] != "Aria Fairwind" || hit["world"] != "Gilgamesh" {
		t.Errorf("hit = %+v; want the exact name back", hit)
	}
	if hit["num_entries"] != float64(3) || hit["payment_state"] != "paid" {
		t.Errorf("hit = %+v; want 3 entries, paid", hit)
	}
	// The waiver is a private arrangement and must not ride along.
	for _, k := range []string{"amount_waived", "id", "total_cost", "amount_due"} {
		if _, present := hit[k]; present {
			t.Errorf("lookup hit leaked %q: %+v", k, hit)
		}
	}
	if body["truncated"] != false {
		t.Errorf("truncated = %v; want false", body["truncated"])
	}

	// One character is a request for the entrant list, not a name search.
	resp = env.postJSON(t, path, map[string]any{"name": "a"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("one-character query: status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestRaffles_EntryLookupHiddenRaffle keeps a raffle that isn't public yet out of
// reach: guessing its id must not make its entrants searchable.
func TestRaffles_EntryLookupHiddenRaffle(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	future := time.Now().UTC().Add(48 * time.Hour).Format("2006-01-02T15:04:05.000Z")
	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Not yet", "max_entries": 3, "available_from": future,
	})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))
	createRaffleEntry(t, env.store, int64(raffleID), "Aria", "Gilgamesh", 1)
	env.signOut(t)

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/lookup", raffleID), map[string]any{"name": "aria"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d; want 404 for a raffle that isn't public yet", resp.StatusCode)
	}
	resp.Body.Close()
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
	_ = env.store.SetRaffleStatus(t.Context(), int64(raffleID), "closed")

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

	// Public sign-up is rejected (past the window)...
	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/enter", raffleID), map[string]any{
		"character_name": "P", "world": "W", "num_entries": 1,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("public enter status = %d; want 400 (window closed)", resp.StatusCode)
	}
	resp.Body.Close()

	// ...but the admin can still add manually.
	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/entries", raffleID), map[string]any{
		"action": "add_entry", "character_name": "P", "world": "W", "num_entries": 1,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("admin add status = %d; want 201 (window ignored)", resp.StatusCode)
	}
	resp.Body.Close()
}

// -- Styles ------------------------------------------------------------------

// -- Settings ----------------------------------------------------------------

// TestRaffles_CloseWithoutWinner is the case verify-winner can't serve: a raffle
// with no entries and no winner - a details-only one whose draw happened
// elsewhere, or simply one nobody entered - still has to be closable, or it stays
// on the public list forever.
func TestRaffles_CloseWithoutWinner(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Info Only", "entry_mode": "details", "max_entries": 3,
	})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))

	// verify-winner refuses, which is what left these stuck.
	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/verify-winner", raffleID), nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("verify-winner with no winner = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/close", raffleID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("close = %d; want 200", resp.StatusCode)
	}
	if decodeBody(t, resp)["status"] != "closed" {
		t.Error("close must report the new status")
	}

	raffle := decodeBody(t, env.get(t, fmt.Sprintf("/api/raffles/%d", raffleID)))["raffle"].(map[string]any)
	if raffle["status"] != "closed" {
		t.Errorf("status = %v; want closed", raffle["status"])
	}
	if raffle["winner_entry_id"] != nil {
		t.Errorf("winner_entry_id = %v; closing must not invent a winner", raffle["winner_entry_id"])
	}

	// Reopen is the undo, and it leaves the raffle exactly as it was.
	resp = env.postJSON(t, fmt.Sprintf("/api/raffles/%d/reopen", raffleID), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reopen = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
	raffle = decodeBody(t, env.get(t, fmt.Sprintf("/api/raffles/%d", raffleID)))["raffle"].(map[string]any)
	if raffle["status"] != "open" {
		t.Errorf("status = %v; want open after reopen", raffle["status"])
	}
}

// TestRaffles_CloseKeepsAnExistingWinner guards the verified-winner result: a
// close (or a stray reopen) must never rewrite who won.
func TestRaffles_CloseKeepsAnExistingWinner(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Drawn", "max_entries": 3, "cost_per_entry": 100,
	})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))
	entryID := createRaffleEntry(t, env.store, int64(raffleID), "Aria", "Gilgamesh", 1)
	env.patchJSON(t, fmt.Sprintf("/api/raffles/%d/entries/%d", raffleID, entryID),
		map[string]any{"paid": true}).Body.Close()
	env.postJSON(t, fmt.Sprintf("/api/raffles/%d/pick-winner", raffleID), nil).Body.Close()
	env.postJSON(t, fmt.Sprintf("/api/raffles/%d/verify-winner", raffleID), nil).Body.Close()

	env.postJSON(t, fmt.Sprintf("/api/raffles/%d/reopen", raffleID), nil).Body.Close()
	env.postJSON(t, fmt.Sprintf("/api/raffles/%d/close", raffleID), nil).Body.Close()

	raffle := decodeBody(t, env.get(t, fmt.Sprintf("/api/raffles/%d", raffleID)))["raffle"].(map[string]any)
	if raffle["winner_entry_id"] != float64(entryID) {
		t.Errorf("winner_entry_id = %v; want the verified winner %d kept", raffle["winner_entry_id"], entryID)
	}
}

// TestRaffles_CloseRequiresPermission keeps the status verbs behind the same
// permission as every other raffle write.
func TestRaffles_CloseRequiresPermission(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	resp := env.postJSON(t, "/api/raffles", map[string]any{"action": "create", "title": "R"})
	raffleID := int(decodeBody(t, resp)["raffle"].(map[string]any)["id"].(float64))
	env.signOut(t)

	for _, verb := range []string{"close", "reopen"} {
		resp := env.postJSON(t, fmt.Sprintf("/api/raffles/%d/%s", raffleID, verb), nil)
		if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s while logged out = %d; want 401/403", verb, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
