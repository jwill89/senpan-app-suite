package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"testing"
)

// passkeyHost is the name passkey requests are made to: WebAuthn takes the site's
// host as the relying party, which must be a name, never the test server's
// 127.0.0.1. The proxy passes the real Host on in production.
const passkeyHost = "example.com"

// hostTransport sends each request as if to host.
type hostTransport struct {
	next http.RoundTripper
	host string
}

func (h hostTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Host = h.host
	return h.next.RoundTrip(r)
}

// passkeyDo sends a JSON request to a passkey operation by env.client, as if to the
// site passkeyHost; a nil body sends none.
func passkeyDo(t *testing.T, e *testEnv, method, path string, body any) *http.Response {
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
	// The Host is set on the way out, so the cookie jar still files the session under
	// the test server's address.
	c := &http.Client{Transport: hostTransport{next: e.client.Transport, host: passkeyHost}, Jar: e.client.Jar}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// TestPasskeyLoginBeginIsRateLimited: every start leaves a WebAuthn challenge in the
// session store until it expires, so starts are counted - on a budget of their own,
// apart from failed sign-ins, so opening the passkey prompt again and again can't
// hold the address back from its password. The 429 says when to try again.
func TestPasskeyLoginBeginIsRateLimited(t *testing.T) {
	env := newTestEnv(t)

	var limited *http.Response
	// The budget is 30 per 10 minutes; go well past it.
	for range 40 {
		resp := passkeyDo(t, env, http.MethodPost, "/api/v1/auth/passkey/begin", nil)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			limited = resp
			break
		}
	}
	if limited == nil {
		t.Fatal("passkey begin never rate limited; unauthenticated callers can mint session challenges without bound")
	}
	if wait, err := strconv.Atoi(limited.Header.Get("Retry-After")); err != nil || wait < 1 {
		t.Errorf("Retry-After = %q; want the seconds to wait", limited.Header.Get("Retry-After"))
	}

	// The failed sign-ins' limit is untouched: the password still signs in.
	env.loginAdmin(t)
}

// TestPasskeySignIn_Ceremony: the options carry a challenge; finishing without a
// start is refused; a finish that doesn't verify is a 401, and the challenge it
// spent answers only once.
func TestPasskeySignIn_Ceremony(t *testing.T) {
	env := newTestEnv(t)

	resp := passkeyDo(t, env, http.MethodPost, "/api/v1/auth/passkey/finish", map[string]any{"credential": map[string]any{}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("finish with nothing started: %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	resp = passkeyDo(t, env, http.MethodPost, "/api/v1/auth/passkey/begin", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("begin: %d; want 200", resp.StatusCode)
	}
	options, _ := decodeBody(t, resp)["publicKey"].(map[string]any)
	if challenge, _ := options["challenge"].(string); challenge == "" {
		t.Fatalf("the options carry no challenge: %v", options)
	}

	resp = passkeyDo(t, env, http.MethodPost, "/api/v1/auth/passkey/finish", map[string]any{"credential": map[string]any{"id": "nope"}})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a finish that doesn't verify: %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
	resp = passkeyDo(t, env, http.MethodPost, "/api/v1/auth/passkey/finish", map[string]any{"credential": map[string]any{"id": "nope"}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("finishing on a spent challenge: %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestPasskeys_ListAddAndRemove: an account's passkeys start empty; adding one asks
// for the account's password and starts with options naming the
// account; a finish that doesn't verify adds nothing; removing an unknown one is a
// 404.
func TestPasskeys_ListAddAndRemove(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	list, _ := decodeBody(t, env.get(t, "/api/v1/me/passkeys"))["passkeys"].([]any)
	if len(list) != 0 {
		t.Fatalf("passkeys = %v; want none", list)
	}

	resp := passkeyDo(t, env, http.MethodPost, "/api/v1/me/passkeys/begin", map[string]string{"password": "wrong"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("begin adding with the wrong password: %d; want 422", resp.StatusCode)
	}
	resp.Body.Close()
	resp = passkeyDo(t, env, http.MethodPost, "/api/v1/me/passkeys/begin", map[string]string{"password": seedAdminPass})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("begin adding: %d; want 200", resp.StatusCode)
	}
	options, _ := decodeBody(t, resp)["publicKey"].(map[string]any)
	user, _ := options["user"].(map[string]any)
	if user["name"] != seedAdminUser {
		t.Errorf("the options name %v; want the account", user["name"])
	}

	resp = passkeyDo(t, env, http.MethodPost, "/api/v1/me/passkeys", map[string]any{"name": "Phone", "credential": map[string]any{"id": "nope"}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("an answer that doesn't verify: %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	if resp := env.del(t, "/api/v1/me/passkeys/12345"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("removing an unknown passkey: %d; want 404", resp.StatusCode)
	}
}

// TestSignInMovesTheSession: signing in moves the session to a new id, so an id
// planted beforehand is worthless.
func TestSignInMovesTheSession(t *testing.T) {
	env := newTestEnv(t)
	jar, _ := cookiejar.New(nil)
	env.client = &http.Client{Transport: env.client.Transport, Jar: jar}

	// Starting a passkey sign-in gives a signed-out visitor a session.
	resp := passkeyDo(t, env, http.MethodPost, "/api/v1/auth/passkey/begin", nil)
	resp.Body.Close()
	before := sessionCookie(t, env)
	if before == "" {
		t.Fatal("starting a passkey sign-in set no session")
	}
	env.loginAdmin(t)
	if after := sessionCookie(t, env); after == "" || after == before {
		t.Errorf("signing in kept the session id (%q before, %q after)", before, after)
	}
}

// TestSignInIsRateLimited: five failed sign-ins from one address hold it back, with
// Retry-After, even for the right password.
func TestSignInIsRateLimited(t *testing.T) {
	env := newTestEnv(t)
	for range 5 {
		resp := env.postJSON(t, "/api/v1/auth/login", map[string]string{"username": seedAdminUser, "password": "wrong"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("a wrong password: %d; want 401", resp.StatusCode)
		}
	}
	resp := env.postJSON(t, "/api/v1/auth/login", map[string]string{"username": seedAdminUser, "password": seedAdminPass})
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("after five failures: %d; want 429", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("the 429 should say when to try again")
	}
}

// sessionCookie is the session cookie env.client holds, or "".
func sessionCookie(t *testing.T, e *testEnv) string {
	t.Helper()
	at, _ := url.Parse(e.ts.URL)
	for _, c := range e.client.Jar.Cookies(at) {
		if c.Name == "session" {
			return c.Value
		}
	}
	return ""
}
