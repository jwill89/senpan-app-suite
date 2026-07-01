package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

// newClient returns a fresh TLS client with its own cookie jar for the test
// server, so a second account can hold an independent session.
func newClient(t *testing.T, e *testEnv) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := e.ts.Client()
	c.Jar = jar
	return c
}

func postAs(t *testing.T, c *http.Client, e *testEnv, path string, body any) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	resp, err := c.Post(e.url(path), "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func getAs(t *testing.T, c *http.Client, e *testEnv, path string) *http.Response {
	t.Helper()
	resp, err := c.Get(e.url(path))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// findUserID looks up a user's id via GET /api/users (admin session on e.client).
func findUserID(t *testing.T, e *testEnv, username string) int64 {
	t.Helper()
	data := decodeBody(t, e.get(t, "/api/users"))
	list, _ := data["users"].([]any)
	for _, it := range list {
		m, _ := it.(map[string]any)
		if m["username"] == username {
			return int64(m["id"].(float64))
		}
	}
	t.Fatalf("user %q not found in users list", username)
	return 0
}

// ── Registration ─────────────────────────────────────────────────────────────

func TestRegister_CreatesInactiveUserThatNeedsActivation(t *testing.T) {
	env := newTestEnv(t)

	resp := env.postJSON(t, "/api/register", map[string]string{"username": "tester", "password": "password123"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Inactive accounts cannot log in yet.
	user := newClient(t, env)
	resp = postAs(t, user, env, "/api/auth", map[string]string{"action": "login", "username": "tester", "password": "password123"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("inactive login status = %d; want 403", resp.StatusCode)
	}
	resp.Body.Close()

	// Admin activates the account, then login succeeds.
	env.loginAdmin(t)
	id := findUserID(t, env, "tester")
	resp = env.patchJSON(t, "/api/users/"+itoa(id), map[string]any{"active": true})
	resp.Body.Close()

	resp = postAs(t, user, env, "/api/auth", map[string]string{"action": "login", "username": "tester", "password": "password123"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("activated login status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRegister_RejectsReservedAndDuplicateAndShort(t *testing.T) {
	env := newTestEnv(t)

	// Reserved "admin" username (seeded) is not registerable.
	resp := env.postJSON(t, "/api/register", map[string]string{"username": "admin", "password": "password123"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("reserved username status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Too-short password.
	resp = env.postJSON(t, "/api/register", map[string]string{"username": "shorty", "password": "short"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("short password status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Duplicate.
	resp = env.postJSON(t, "/api/register", map[string]string{"username": "dup", "password": "password123"})
	resp.Body.Close()
	resp = env.postJSON(t, "/api/register", map[string]string{"username": "dup", "password": "password123"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate username status = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()
}

// ── Permission enforcement ───────────────────────────────────────────────────

// makeActiveUser registers a user, then (as admin) activates it and grants the
// given page permissions. Returns a logged-in client for that user.
func makeActiveUser(t *testing.T, env *testEnv, username, password string, perms []string) *http.Client {
	t.Helper()
	resp := env.postJSON(t, "/api/register", map[string]string{"username": username, "password": password})
	resp.Body.Close()

	env.loginAdmin(t)
	id := findUserID(t, env, username)
	resp = env.patchJSON(t, "/api/users/"+itoa(id), map[string]any{"active": true})
	resp.Body.Close()
	resp = env.patchJSON(t, "/api/users/"+itoa(id), map[string]any{"permissions": perms})
	resp.Body.Close()

	c := newClient(t, env)
	resp = postAs(t, c, env, "/api/auth", map[string]string{"action": "login", "username": username, "password": password})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login for %q status = %d; want 200", username, resp.StatusCode)
	}
	resp.Body.Close()
	return c
}

func TestPermission_GrantedAllowedOthersForbidden(t *testing.T) {
	env := newTestEnv(t)
	user := makeActiveUser(t, env, "host", "password123", []string{"bingo-cards"})

	// Granted page: creating a card works (POST /api/cards → 201).
	resp := postAs(t, user, env, "/api/cards", map[string]any{"player_name": "Guest"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("granted cards create status = %d; want 201", resp.StatusCode)
	}
	resp.Body.Close()

	// Ungranted page: settings update is forbidden (not just hidden in the UI).
	resp = postAs(t, user, env, "/api/settings", map[string]any{"settings": map[string]string{"app_title": "Hax"}})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("ungranted settings update status = %d; want 403", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPermission_AdminBypassesEverything(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/settings", map[string]any{"settings": map[string]string{"app_title": "Hi"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin settings update status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestUsers_RequireAdmin(t *testing.T) {
	env := newTestEnv(t)
	user := makeActiveUser(t, env, "plain", "password123", []string{"bingo-cards"})

	resp := getAs(t, user, env, "/api/users")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("non-admin GET /api/users status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

// ── Protected admin account ──────────────────────────────────────────────────

func TestProtectedAdmin_CannotBeModifiedByAdmins(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	adminID := findUserID(t, env, "admin")
	path := "/api/users/" + itoa(adminID)

	// PATCHing active/admin/password on the protected admin account is refused.
	for _, body := range []map[string]any{
		{"active": false},
		{"admin": false},
		{"password": "password123"},
	} {
		resp := env.patchJSON(t, path, body)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("PATCH %v on admin: status = %d; want 403", body, resp.StatusCode)
		}
		resp.Body.Close()
	}

	// DELETE of the protected admin account is refused.
	resp := env.del(t, path)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("DELETE admin: status = %d; want 403", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestProtectedAdmin_PermissionsStillEditable confirms setting permissions on the
// admin account is allowed (deliberately not part of the protected field set —
// admins hold every permission implicitly, so it's a harmless no-op here).
func TestProtectedAdmin_PermissionsStillEditable(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	adminID := findUserID(t, env, "admin")

	resp := env.patchJSON(t, "/api/users/"+itoa(adminID), map[string]any{"permissions": []string{"bingo-cards"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH permissions on admin: status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

// ── Self-service password change ─────────────────────────────────────────────

func TestAccount_ChangePassword(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Wrong current password is rejected.
	resp := env.postJSON(t, "/api/account/change-password", map[string]string{
		"current_password": "nope", "new_password": "password123",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong current password status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()

	// Correct change succeeds.
	resp = env.postJSON(t, "/api/account/change-password", map[string]string{
		"current_password": seedAdminPass, "new_password": "newpassword1",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("change password status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Old password no longer works; new one does (fresh client).
	c := newClient(t, env)
	resp = postAs(t, c, env, "/api/auth", map[string]string{"action": "login", "username": seedAdminUser, "password": seedAdminPass})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password login status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
	resp = postAs(t, c, env, "/api/auth", map[string]string{"action": "login", "username": seedAdminUser, "password": "newpassword1"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("new password login status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
}
