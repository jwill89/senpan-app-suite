package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"app-suite/internal/store"
)

func TestTurnstileEnabled(t *testing.T) {
	if (&Server{}).turnstileEnabled() {
		t.Error("no secret should mean disabled")
	}
	if !(&Server{turnstileSecret: "x"}).turnstileEnabled() {
		t.Error("a secret should mean enabled")
	}
}

// stubSiteverify returns a Turnstile siteverify stub that approves only the token
// "good" (and only with the expected secret).
func stubSiteverify(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		ok := r.FormValue("secret") == "sekret" && r.FormValue("response") == "good"
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": ok})
	}))
	prev := turnstileVerifyURL
	turnstileVerifyURL = srv.URL
	t.Cleanup(func() { turnstileVerifyURL = prev; srv.Close() })
	return srv
}

func TestVerifyTurnstile(t *testing.T) {
	stubSiteverify(t)
	s := &Server{turnstileSecret: "sekret"}

	if !s.verifyTurnstile(context.Background(), "good", "1.2.3.4") {
		t.Error("valid token should verify")
	}
	if s.verifyTurnstile(context.Background(), "bad", "") {
		t.Error("invalid token should fail")
	}
	if s.verifyTurnstile(context.Background(), "  ", "") {
		t.Error("blank token should fail without a request")
	}
}

func TestVerifyTurnstileFailsClosed(t *testing.T) {
	prev := turnstileVerifyURL
	turnstileVerifyURL = "http://127.0.0.1:0/unreachable"
	defer func() { turnstileVerifyURL = prev }()

	if (&Server{turnstileSecret: "sekret"}).verifyTurnstile(context.Background(), "good", "") {
		t.Error("a transport error must fail closed (reject), not pass")
	}
}

// TestLoginGatedByTurnstile checks the bot gate runs before any credential work:
// a missing/invalid token is rejected with 403; a valid token lets the request
// reach the credential check (401 for an unknown user).
func TestLoginGatedByTurnstile(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	stubSiteverify(t)

	s := &Server{store: st, limiter: newRateLimiter(5, time.Minute), turnstileSecret: "sekret"}

	post := func(body map[string]string) int {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/auth", bytes.NewReader(b))
		rec := httptest.NewRecorder()
		s.handleAuthAction(rec, req)
		return rec.Code
	}

	if code := post(map[string]string{"action": "login", "username": "admin", "password": "x"}); code != http.StatusForbidden {
		t.Errorf("missing token: got %d, want 403", code)
	}
	if code := post(map[string]string{"action": "login", "username": "admin", "password": "x", "turnstile_token": "bad"}); code != http.StatusForbidden {
		t.Errorf("bad token: got %d, want 403", code)
	}
	if code := post(map[string]string{"action": "login", "username": "ghost", "password": "x", "turnstile_token": "good"}); code != http.StatusUnauthorized {
		t.Errorf("good token should pass the gate and 401 on bad creds: got %d, want 401", code)
	}
}

// TestLoginUngatedWhenDisabled confirms that with no secret the bot check is
// skipped entirely (a login with no token reaches the credential check).
func TestLoginUngatedWhenDisabled(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	s := &Server{store: st, limiter: newRateLimiter(5, time.Minute)} // no turnstileSecret
	b, _ := json.Marshal(map[string]string{"action": "login", "username": "ghost", "password": "x"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth", bytes.NewReader(b))
	rec := httptest.NewRecorder()
	s.handleAuthAction(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("disabled gate: got %d, want 401 (reached credential check)", rec.Code)
	}
}

// TestRegisterGatedByTurnstile checks the bot gate also guards account creation:
// no token → 403; a valid token lets a valid signup through (200).
func TestRegisterGatedByTurnstile(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	stubSiteverify(t)

	s := &Server{store: st, regLimiter: newRateLimiter(10, time.Hour), turnstileSecret: "sekret"}

	post := func(body map[string]string) int {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/register", bytes.NewReader(b))
		rec := httptest.NewRecorder()
		s.handleRegister(rec, req)
		return rec.Code
	}

	if code := post(map[string]string{"username": "newuser", "password": "password123"}); code != http.StatusForbidden {
		t.Errorf("missing token: got %d, want 403", code)
	}
	if code := post(map[string]string{"username": "newuser", "password": "password123", "turnstile_token": "good"}); code != http.StatusOK {
		t.Errorf("good token should create the account: got %d, want 200", code)
	}
}

func TestHandleConfigServesSiteKey(t *testing.T) {
	s := &Server{turnstileSiteKey: "site-abc"}
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	s.handleConfig(rec, req)

	var out struct {
		TurnstileSiteKey string `json:"turnstile_site_key"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.TurnstileSiteKey != "site-abc" {
		t.Errorf("site key: got %q, want site-abc", out.TurnstileSiteKey)
	}
}
