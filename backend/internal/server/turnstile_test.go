package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
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
	s, _, ts := newInternalServer(t)
	stubSiteverify(t)
	s.turnstileSecret = "sekret"

	post := func(body map[string]string) int {
		code, _ := postFrom(t, ts, "/api/v1/auth/login", body)
		return code
	}
	if code := post(map[string]string{"username": "admin", "password": "x"}); code != http.StatusForbidden {
		t.Errorf("missing token: got %d, want 403", code)
	}
	if code := post(map[string]string{"username": "admin", "password": "x", "turnstileToken": "bad"}); code != http.StatusForbidden {
		t.Errorf("bad token: got %d, want 403", code)
	}
	if code := post(map[string]string{"username": "ghost", "password": "x", "turnstileToken": "good"}); code != http.StatusUnauthorized {
		t.Errorf("good token should pass the gate and 401 on bad creds: got %d, want 401", code)
	}
}

// TestLoginUngatedWhenDisabled confirms that with no secret the bot check is
// skipped entirely (a login with no token reaches the credential check).
func TestLoginUngatedWhenDisabled(t *testing.T) {
	_, _, ts := newInternalServer(t)
	if code, _ := postFrom(t, ts, "/api/v1/auth/login", map[string]string{"username": "ghost", "password": "x"}); code != http.StatusUnauthorized {
		t.Errorf("disabled gate: got %d, want 401 (reached credential check)", code)
	}
}

// TestRegisterGatedByTurnstile checks the bot gate also guards account creation:
// no token -> 403; a valid token lets a valid signup through (202).
func TestRegisterGatedByTurnstile(t *testing.T) {
	s, _, ts := newInternalServer(t)
	stubSiteverify(t)
	s.turnstileSecret = "sekret"

	post := func(body map[string]string) int {
		code, _ := postFrom(t, ts, "/api/v1/auth/register", body)
		return code
	}
	if code := post(map[string]string{"username": "newuser", "password": "password123"}); code != http.StatusForbidden {
		t.Errorf("missing token: got %d, want 403", code)
	}
	if code := post(map[string]string{"username": "newuser", "password": "password123", "turnstileToken": "good"}); code != http.StatusAccepted {
		t.Errorf("good token should create the account: got %d, want 202", code)
	}
}

// TestCardRequestGatedByTurnstile checks the bot gate guards the public personal
// card request: no token or a bad one -> 403 and nothing kept; a valid token lets
// the request through (202).
func TestCardRequestGatedByTurnstile(t *testing.T) {
	s, st, ts := newInternalServer(t)
	stubSiteverify(t)
	s.turnstileSecret = "sekret"

	post := func(token string) int {
		body := map[string]any{
			"characterName": "Aria", "world": "Gilgamesh", "cardId": "BOT001",
			"boardData": [][]int{{1, 16, 31, 46, 61}, {2, 17, 32, 47, 62}, {3, 18, 0, 48, 63}, {4, 19, 34, 49, 64}, {5, 20, 35, 50, 65}},
		}
		if token != "" {
			body["turnstileToken"] = token
		}
		code, _ := postFrom(t, ts, "/api/v1/cards/request", body)
		return code
	}
	if code := post(""); code != http.StatusForbidden {
		t.Errorf("missing token: got %d, want 403", code)
	}
	if code := post("bad"); code != http.StatusForbidden {
		t.Errorf("bad token: got %d, want 403", code)
	}
	if kept, _ := st.CardExists(t.Context(), "BOT001"); kept {
		t.Fatal("a request that failed the bot check was kept")
	}
	if code := post("good"); code != http.StatusAccepted {
		t.Errorf("good token should take the request in: got %d, want 202", code)
	}
}

// TestRaffleEnterGatedByTurnstile checks the bot gate guards the public raffle
// sign-up: no token or a bad one -> 403 and no entry kept; a valid token lets the
// sign-up through (201).
func TestRaffleEnterGatedByTurnstile(t *testing.T) {
	s, st, ts := newInternalServer(t)
	stubSiteverify(t)
	s.turnstileSecret = "sekret"
	res, err := st.DB().ExecContext(t.Context(), `INSERT INTO raffles (title, max_entries) VALUES ('Gated', 3)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	post := func(token string) int {
		body := map[string]any{"characterName": "Aria", "world": "Gilgamesh", "numEntries": 1}
		if token != "" {
			body["turnstileToken"] = token
		}
		code, _ := postFrom(t, ts, "/api/v1/raffles/"+strconv.FormatInt(id, 10)+"/enter", body)
		return code
	}
	if code := post(""); code != http.StatusForbidden {
		t.Errorf("missing token: got %d, want 403", code)
	}
	if code := post("bad"); code != http.StatusForbidden {
		t.Errorf("bad token: got %d, want 403", code)
	}
	var entries int
	if err := st.DB().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM raffle_entries`).Scan(&entries); err != nil || entries != 0 {
		t.Fatalf("a sign-up that failed the bot check was kept: %d entries, %v", entries, err)
	}
	if code := post("good"); code != http.StatusCreated {
		t.Errorf("good token should take the sign-up in: got %d, want 201", code)
	}
}
