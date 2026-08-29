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

// -- Registration -------------------------------------------------------------

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

func TestRegister_RejectsShortPassword(t *testing.T) {
	env := newTestEnv(t)

	// Too-short password is rejected before any account is created. (Reserved and
	// duplicate usernames are deliberately NOT rejected with a distinct status -
	// they return the same generic 200 to prevent enumeration; see
	// TestRegisterNoEnumeration.)
	resp := env.postJSON(t, "/api/register", map[string]string{"username": "shorty", "password": "short"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("short password status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// -- Permission enforcement ---------------------------------------------------

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

	// Granted page: creating a card works (POST /api/cards -> 201).
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

// -- Protected admin account --------------------------------------------------

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
// admin account is allowed (deliberately not part of the protected field set -
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

// -- Self-service password change ---------------------------------------------

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

// TestLoginAfterPasswordChangeStampsEpoch guards the session contract that every
// login path now shares via establishSession. A session carries the account's
// password epoch as well as its id; one minted without the epoch reads back as
// epoch 0, which loadCurrentUser rejects for any account whose password has ever
// changed. The failure is nasty precisely because the login itself looks fine -
// it returns 200 with the user object, and only the NEXT request 401s. That is
// what a passkey login did for every account past epoch 0, because it stamped the
// id and not the epoch.
func TestLoginAfterPasswordChangeStampsEpoch(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Bump the account past epoch 0. Below this point a session missing the epoch
	// is indistinguishable from a stale one and gets rejected.
	resp := env.postJSON(t, "/api/account/change-password", map[string]string{
		"current_password": seedAdminPass, "new_password": "newpassword1",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("change password status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	c := newClient(t, env)
	resp = postAs(t, c, env, "/api/auth", map[string]string{
		"action": "login", "username": seedAdminUser, "password": "newpassword1",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// The assertion that matters: the session survives into the next request.
	resp = getAs(t, c, env, "/api/auth")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("auth check after login status = %d; want 200", resp.StatusCode)
	}
	if authed, _ := decodeBody(t, resp)["authenticated"].(bool); !authed {
		t.Error("session minted by login does not resolve on the next request (password epoch not stamped)")
	}
}

// TestPermission_EveryPageGuardsAReadRoute is the coverage the suite was missing.
// Only a handful of the grantable permission keys had ANY negative test, so a guard
// silently dropped or loosened on the other features would have gone unnoticed -
// and the whole point of per-page permissions is that a grantee of one page cannot
// reach another.
//
// One representative READ route per page: a GET is enough to prove the guard is
// wired, needs no fixture, and leaves nothing behind if the guard is broken. Every
// route listed here was confirmed against its handler to be permission-gated;
// deliberately public reads are NOT listed, since asserting 403 on them would be
// asserting the wrong thing: GET /api/game is what every player's board polls, and
// GET /api/raffles is the public raffle list, which varies its payload by role
// (raffleStaff) rather than refusing outright.
//
// ONE user for the whole table on purpose. Registration is rate limited to 5 per
// hour per IP, and newClient hands back the httptest server's SHARED *http.Client
// with only the cookie jar swapped - so juggling several "separate" clients in one
// test really means one client whose last login wins.
func TestPermission_EveryPageGuardsAReadRoute(t *testing.T) {
	routes := map[string]string{
		"bingo-cards":            "/api/cards",
		"bingo-winners-log":      "/api/winners-log",
		"bingo-patterns":         "/api/patterns",
		"teahouse-announcements": "/api/announcements",
		"teahouse-affiliates":    "/api/affiliates",
		"teahouse-tea-rooms":     "/api/tea-rooms",
		"festival-map":           "/api/festival-maps",
		"festival-garapon":       "/api/garapons",
		"festival-stamp-rally":   "/api/stamp-rallies",
		"atelier-fonts":          "/api/fonts",
		"system-themes":          "/api/styles",
		"system-images":          "/api/images?dir=announcements",
		"bookclub-yaoi":          "/api/book-clubs/yaoi/reading-lists",
		"bookclub-yuri":          "/api/book-clubs/yuri/reading-lists",
	}

	env := newTestEnv(t)
	// Holds exactly one page, and it is not any of the ones under test - so every
	// assertion below is asserting a refusal.
	user := makeActiveUser(t, env, "onepage", "correct horse battery", []string{"bingo-presets"})

	for perm, route := range routes {
		resp := getAs(t, user, env, route)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("GET %s without %q = %d; want 403 - that page's guard is missing or too loose",
				route, perm, resp.StatusCode)
		}
	}

	// The page they DO hold stays reachable, so the table above is proving a guard
	// rather than an endpoint that is broken for everyone.
	resp := getAs(t, user, env, "/api/presets")
	resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		t.Errorf("GET /api/presets WITH bingo-presets = %d; the grantee cannot reach their own page",
			resp.StatusCode)
	}
}
