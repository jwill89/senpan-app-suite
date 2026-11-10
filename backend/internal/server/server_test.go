package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"app-suite/internal/api/apitest"
	"app-suite/internal/live"
	"app-suite/internal/server"
	"app-suite/internal/store"
	"app-suite/internal/store/storetest"
)

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

	resp := env.get(t, "/api/stamp-card/nosuchtoken")
	data := decodeBody(t, resp)

	if _, ok := data["error"]; !ok {
		t.Error("expected error key in JSON error response")
	}
}

func TestJSONContentType(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/stamp-signup")
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q; want application/json; charset=utf-8", ct)
	}
}

func TestCacheControlNoStore(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/stamp-signup")
	defer resp.Body.Close()

	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q; want no-store", cc)
	}
}
