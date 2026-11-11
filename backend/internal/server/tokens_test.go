package server_test

import (
	"net/http"
	"strings"
	"testing"

	"app-suite/internal/auth"
	"app-suite/internal/db"
)

// bearerDo sends a request carrying a token on a client with no cookie jar, on the
// test server's transport, so only the token can sign it in.
func bearerDo(t *testing.T, e *testEnv, method, path, token string) *http.Response {
	t.Helper()
	c := &http.Client{Transport: e.client.Transport}
	req, _ := http.NewRequest(method, e.url(path), nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func bearerGet(t *testing.T, e *testEnv, path, token string) *http.Response {
	t.Helper()
	return bearerDo(t, e, http.MethodGet, path, token)
}

// createToken makes c's account a token and answers its plaintext.
func createToken(t *testing.T, c *http.Client, e *testEnv) string {
	t.Helper()
	resp := reqAs(t, c, e, http.MethodPost, "/api/v1/me/token", nil)
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create a token: %d; want 201", resp.StatusCode)
	}
	token, _ := decodeBody(t, resp)["token"].(string)
	if !strings.HasPrefix(token, auth.TokenScheme) {
		t.Fatalf("token = %q; want a %s one", token, auth.TokenScheme)
	}
	return token
}

// TestAccountToken_GenerateInfoRevoke covers the lifecycle: an account has no token,
// makes one (shown once), sees only its summary afterward, uses it, then revokes it.
func TestAccountToken_GenerateInfoRevoke(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	data := decodeBody(t, env.get(t, "/api/v1/me/token"))
	if tok, present := data["token"]; !present || tok != nil {
		t.Fatalf("token = %v before one is made; want null", tok)
	}

	token := createToken(t, env.client, env)

	// The summary shows the token by its prefix - and never its secret.
	summary, _ := decodeBody(t, env.get(t, "/api/v1/me/token"))["token"].(map[string]any)
	prefix, _ := summary["prefix"].(string)
	if prefix == "" || !strings.HasPrefix(token, prefix) || prefix == token {
		t.Fatalf("summary prefix %q isn't a short prefix of the token", prefix)
	}
	if summary["createdAt"] == nil {
		t.Error("the summary should say when the token was made")
	}

	// The token signs in as the admin on an admin-only operation.
	if resp := bearerGet(t, env, "/api/v1/users", token); resp.StatusCode != http.StatusOK {
		t.Fatalf("token GET /api/v1/users = %d; want 200", resp.StatusCode)
	}

	// Revoked, it stops working at once; revoking again finds nothing.
	if resp := env.del(t, "/api/v1/me/token"); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke: %d; want 204", resp.StatusCode)
	}
	if resp := bearerGet(t, env, "/api/v1/users", token); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked token GET /api/v1/users = %d; want 401", resp.StatusCode)
	}
	if resp := env.del(t, "/api/v1/me/token"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("revoke again: %d; want 404", resp.StatusCode)
	}
}

// TestAccountToken_ReplacesAndNotesUse: making a token again replaces the first, and
// using one notes when.
func TestAccountToken_ReplacesAndNotesUse(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	first := createToken(t, env.client, env)
	second := createToken(t, env.client, env)

	if resp := bearerGet(t, env, "/api/v1/users", first); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the replaced token: %d; want 401", resp.StatusCode)
	}
	summary, _ := decodeBody(t, env.get(t, "/api/v1/me/token"))["token"].(map[string]any)
	if summary["lastUsedAt"] != nil {
		t.Errorf("lastUsedAt = %v before the token is used; want null", summary["lastUsedAt"])
	}
	if resp := bearerGet(t, env, "/api/v1/users", second); resp.StatusCode != http.StatusOK {
		t.Fatalf("the new token: %d; want 200", resp.StatusCode)
	}
	summary, _ = decodeBody(t, env.get(t, "/api/v1/me/token"))["token"].(map[string]any)
	if summary["lastUsedAt"] == nil {
		t.Error("lastUsedAt should be set once the token is used")
	}
}

// TestTokenAuth_RejectsMissingAndBogus verifies the token path doesn't weaken the
// guards: no credentials and an unknown token both 401.
func TestTokenAuth_RejectsMissingAndBogus(t *testing.T) {
	env := newTestEnv(t)

	if resp := bearerGet(t, env, "/api/v1/users", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-auth GET /api/v1/users = %d; want 401", resp.StatusCode)
	}
	if resp := bearerGet(t, env, "/api/v1/users", "pat_this-token-does-not-exist"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bogus-token GET /api/v1/users = %d; want 401", resp.StatusCode)
	}
}

// TestTokenAuth_EnforcesUserPermissions confirms a token carries exactly its
// owner's access: per-page permissions hold, and it is no admin.
func TestTokenAuth_EnforcesUserPermissions(t *testing.T) {
	env := newTestEnv(t)
	addAccount(t, env, "plugin", "password123", []string{"bingo-cards"})
	user := newClient(t, env)
	signInAs(t, user, env, "plugin", "password123")
	token := createToken(t, user, env)

	if resp := bearerGet(t, env, "/api/v1/cards", token); resp.StatusCode != http.StatusOK {
		t.Fatalf("granted page via the token = %d; want 200", resp.StatusCode)
	}
	if resp := bearerGet(t, env, "/api/v1/users", token); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("admin-only page via the token = %d; want 403", resp.StatusCode)
	}
	if resp := bearerGet(t, env, "/api/v1/me", token); accountName(t, resp) != "plugin" {
		t.Error("who am I, by the token, should be its account")
	}
}

// TestToken_CannotChangeCredentials: a token can't make or revoke tokens, change the
// password or manage passkeys; those take the account's session.
func TestToken_CannotChangeCredentials(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	token := createToken(t, env.client, env)

	for _, op := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/me/token"},
		{http.MethodDelete, "/api/v1/me/token"},
		{http.MethodPut, "/api/v1/me/password"},
		{http.MethodPost, "/api/v1/me/passkeys/begin"},
		{http.MethodDelete, "/api/v1/me/passkeys/1"},
	} {
		resp := bearerDo(t, env, op.method, op.path, token)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s by a token = %d; want 403", op.method, op.path, resp.StatusCode)
		}
	}
	if resp := bearerGet(t, env, "/api/v1/users", token); resp.StatusCode != http.StatusOK {
		t.Errorf("the token itself should still work: %d", resp.StatusCode)
	}
}

// TestToken_InactiveAccountDoesNotResolve: deactivating an account stops its token
// at once.
func TestToken_InactiveAccountDoesNotResolve(t *testing.T) {
	env := newTestEnv(t)
	id := addAccount(t, env, "plugin", "password123", []string{"bingo-cards"})
	user := newClient(t, env)
	signInAs(t, user, env, "plugin", "password123")
	token := createToken(t, user, env)

	if err := db.New(env.store.DB()).UpdateUser(t.Context(), db.UpdateUserParams{
		IsActive: 0, IsAdmin: 0, Permissions: `["bingo-cards"]`, ID: id,
	}); err != nil {
		t.Fatal(err)
	}
	if resp := bearerGet(t, env, "/api/v1/cards", token); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a deactivated account's token: %d; want 401", resp.StatusCode)
	}
}
