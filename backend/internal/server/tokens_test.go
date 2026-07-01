package server_test

import (
	"net/http"
	"strings"
	"testing"
)

// bearerGet performs a GET carrying a PAT bearer token on a fresh, cookieless
// client (its own http.Client with no jar, reusing the test server's TLS-trusting
// transport). This exercises the token-auth path in isolation — no session cookie
// can leak in from env.client.
func bearerGet(t *testing.T, e *testEnv, path, token string) *http.Response {
	t.Helper()
	c := &http.Client{Transport: e.ts.Client().Transport}
	req, _ := http.NewRequest("GET", e.url(path), nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// TestAccountToken_GenerateInfoRevoke covers the self-service lifecycle: an
// account has no token, generates one (revealed exactly once), sees only its
// metadata afterward, uses it to authenticate, then revokes it.
func TestAccountToken_GenerateInfoRevoke(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// No token to start.
	if info := decodeBody(t, env.get(t, "/api/account/token")); info["has_token"] != false {
		t.Fatalf("has_token = %v; want false before generation", info["has_token"])
	}

	// Generate: the plaintext token is returned exactly once, prefix-tagged.
	gen := decodeBody(t, env.postJSON(t, "/api/account/token", map[string]string{"action": "generate"}))
	token, _ := gen["token"].(string)
	if !strings.HasPrefix(token, "pat_") {
		t.Fatalf("generated token = %q; want a pat_-prefixed value", token)
	}
	if prefix, _ := gen["prefix"].(string); prefix == "" || !strings.HasPrefix(token, prefix) {
		t.Fatalf("prefix %q is not a non-empty prefix of the token", gen["prefix"])
	}

	// Info now reports the token exists — but never echoes the secret itself.
	info := decodeBody(t, env.get(t, "/api/account/token"))
	if info["has_token"] != true {
		t.Fatalf("has_token = %v; want true after generation", info["has_token"])
	}
	if _, leaked := info["token"]; leaked {
		t.Fatal("GET /api/account/token must not echo the token plaintext")
	}

	// The token authenticates as the admin on an admin-only endpoint.
	resp := bearerGet(t, env, "/api/users", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token-authed GET /api/users = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Revoke (DELETE): the same token stops working immediately.
	env.del(t, "/api/account/token").Body.Close()
	resp = bearerGet(t, env, "/api/users", token)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked-token GET /api/users = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestTokenAuth_RejectsMissingAndBogus verifies the token path doesn't weaken the
// guards: no credentials and an unknown token both 401.
func TestTokenAuth_RejectsMissingAndBogus(t *testing.T) {
	env := newTestEnv(t)

	resp := bearerGet(t, env, "/api/users", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-auth GET /api/users = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()

	resp = bearerGet(t, env, "/api/users", "pat_this-token-does-not-exist")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bogus-token GET /api/users = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestTokenAuth_EnforcesUserPermissions confirms a token carries exactly its
// owner's access — the headline guarantee that logging in via PAT still respects
// per-page permissions (a token-holder can't exceed what the account may do).
func TestTokenAuth_EnforcesUserPermissions(t *testing.T) {
	env := newTestEnv(t)

	// Register a user, then (as admin) activate it and grant only bingo-cards.
	// All admin operations happen on env.client before the user client is created.
	env.postJSON(t, "/api/register", map[string]string{"username": "plugin", "password": "password123"}).Body.Close()
	env.loginAdmin(t)
	id := findUserID(t, env, "plugin")
	env.patchJSON(t, "/api/users/"+itoa(id), map[string]any{"active": true}).Body.Close()
	env.patchJSON(t, "/api/users/"+itoa(id), map[string]any{"permissions": []string{"bingo-cards"}}).Body.Close()

	// Log in as that user (independent client/jar) and mint a token.
	user := newClient(t, env)
	postAs(t, user, env, "/api/auth", map[string]string{"action": "login", "username": "plugin", "password": "password123"}).Body.Close()
	gen := decodeBody(t, postAs(t, user, env, "/api/account/token", map[string]string{"action": "generate"}))
	token, _ := gen["token"].(string)
	if token == "" {
		t.Fatal("expected a generated token for the plugin user")
	}

	// Granted page → 200 via the token.
	resp := bearerGet(t, env, "/api/cards", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("granted endpoint via token = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Admin-only page → rejected (the token is not admin).
	resp = bearerGet(t, env, "/api/users", token)
	if resp.StatusCode == http.StatusOK {
		t.Fatal("admin-only endpoint must reject a non-admin token")
	}
	resp.Body.Close()
}
