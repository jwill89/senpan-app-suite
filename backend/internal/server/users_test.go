package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"testing"

	"app-suite/internal/auth"
	"app-suite/internal/db"
)

// newClient returns a client of its own - its own cookie jar on the test server's
// transport, which checks every answer against the document - so a second account
// holds a session apart from env.client's.
func newClient(t *testing.T, e *testEnv) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &http.Client{Transport: e.client.Transport, Jar: jar}
}

// reqAs sends a JSON request with any method as the client's account; a nil body
// sends none.
func reqAs(t *testing.T, c *http.Client, e *testEnv, method, path string, body any) *http.Response {
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
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
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

// signInAs signs c in and fails the test unless it worked.
func signInAs(t *testing.T, c *http.Client, e *testEnv, username, password string) {
	t.Helper()
	resp := postAs(t, c, e, "/api/v1/auth/login", map[string]string{"username": username, "password": password})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sign-in as %q: %d; want 200", username, resp.StatusCode)
	}
}

// signIn signs env.client in.
func (e *testEnv) signIn(t *testing.T, username, password string) {
	t.Helper()
	signInAs(t, e.client, e, username, password)
}

// addAccount makes an active, non-admin account holding perms straight in the
// database, and answers its id.
func addAccount(t *testing.T, e *testEnv, username, password string, perms []string) int64 {
	t.Helper()
	hash, err := auth.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	q := db.New(e.store.DB())
	if err := q.CreateUser(t.Context(), db.CreateUserParams{Username: username, PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}
	row, err := q.GetUserByUsername(t.Context(), username)
	if err != nil {
		t.Fatal(err)
	}
	setAccess(t, e, row.ID, perms)
	return row.ID
}

// setAccess makes an account active, not an admin, and holding exactly perms,
// straight in the database. A guard reads the account again on every request, so
// the next request already meets the change.
func setAccess(t *testing.T, e *testEnv, id int64, perms []string) {
	t.Helper()
	if err := db.New(e.store.DB()).UpdateUser(t.Context(), db.UpdateUserParams{
		IsActive: 1, IsAdmin: 0, Permissions: auth.EncodePermissions(perms), ID: id,
	}); err != nil {
		t.Fatal(err)
	}
}

// findUserID looks up a user's id via GET /api/v1/users (admin session on e.client).
func findUserID(t *testing.T, e *testEnv, username string) int64 {
	t.Helper()
	data := decodeBody(t, e.get(t, "/api/v1/users"))
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

	resp := env.postJSON(t, "/api/v1/auth/register", map[string]string{"username": "tester", "password": "password123"})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("register status = %d; want 202", resp.StatusCode)
	}
	resp.Body.Close()

	// Inactive accounts cannot log in yet.
	user := newClient(t, env)
	resp = postAs(t, user, env, "/api/v1/auth/login", map[string]string{"username": "tester", "password": "password123"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("inactive login status = %d; want 403", resp.StatusCode)
	}
	resp.Body.Close()

	// Admin activates the account, then login succeeds.
	env.loginAdmin(t)
	id := findUserID(t, env, "tester")
	resp = env.patchJSON(t, "/api/v1/users/"+itoa(id), map[string]any{"isActive": true})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("activate status = %d; want 200", resp.StatusCode)
	}
	if active, _ := decodeBody(t, resp)["isActive"].(bool); !active {
		t.Error("the answer should show the account active")
	}
	signInAs(t, user, env, "tester", "password123")
}

func TestRegister_RejectsShortPassword(t *testing.T) {
	env := newTestEnv(t)

	// A short password is refused before any account is made. (A reserved or taken
	// name is NOT refused, so as not to say who has an account: see
	// TestRegisterNoEnumeration.)
	resp := env.postJSON(t, "/api/v1/auth/register", map[string]string{"username": "shorty", "password": "short"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("short password status = %d; want 422", resp.StatusCode)
	}
	resp.Body.Close()
}

// -- Permission enforcement ---------------------------------------------------

// makeActiveUser signs up a user, then (as admin, on env.client) activates it and
// grants the given page permissions. Returns a client of its own signed in as it.
func makeActiveUser(t *testing.T, env *testEnv, username, password string, perms []string) *http.Client {
	t.Helper()
	resp := env.postJSON(t, "/api/v1/auth/register", map[string]string{"username": username, "password": password})
	resp.Body.Close()

	env.loginAdmin(t)
	id := findUserID(t, env, username)
	resp = env.patchJSON(t, "/api/v1/users/"+itoa(id), map[string]any{"isActive": true, "permissions": perms})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set up %q: %d", username, resp.StatusCode)
	}

	c := newClient(t, env)
	signInAs(t, c, env, username, password)
	return c
}

func TestPermission_GrantedAllowedOthersForbidden(t *testing.T) {
	env := newTestEnv(t)
	user := makeActiveUser(t, env, "host", "password123", []string{"bingo-cards"})

	// Granted page: creating a card works (POST /api/v1/cards -> 201).
	resp := postAs(t, user, env, "/api/v1/cards", map[string]any{"playerName": "Guest"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("granted cards create status = %d; want 201", resp.StatusCode)
	}
	resp.Body.Close()

	// Ungranted page: settings update is forbidden (not just hidden in the UI).
	resp = reqAs(t, user, env, http.MethodPut, "/api/v1/settings", map[string]any{"appTitle": "Hax"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("ungranted settings update status = %d; want 403", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPermission_AdminBypassesEverything(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := saveSettings(t, env, map[string]any{"appTitle": "Hi"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin settings update status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestUsers_RequireAdmin(t *testing.T) {
	env := newTestEnv(t)
	user := makeActiveUser(t, env, "plain", "password123", []string{"bingo-cards"})

	resp := getAs(t, user, env, "/api/v1/users")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin GET /api/v1/users status = %d; want 403", resp.StatusCode)
	}
	resp.Body.Close()

	resp = getAs(t, newClient(t, env), env, "/api/v1/users")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("signed-out GET /api/v1/users status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestUsers_RefuseUnknownPermissions(t *testing.T) {
	env := newTestEnv(t)
	id := addAccount(t, env, "staff", "password123", nil)
	env.loginAdmin(t)

	resp := env.patchJSON(t, "/api/v1/users/"+itoa(id), map[string]any{"permissions": []string{"bingo-cards", "no-such-page"}})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown permission: %d; want 422", resp.StatusCode)
	}
	errs, _ := decodeBody(t, resp)["errors"].([]any)
	first, _ := errs[0].(map[string]any)
	if len(errs) != 1 || first["location"] != "body.permissions[1]" {
		t.Errorf("errors = %v; want the second permission named", errs)
	}

	resp = env.patchJSON(t, "/api/v1/users/999999", map[string]any{"isActive": true})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown account: %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestUsers_DeleteEndsTheAccount: a deleted account's session acts as a guest and
// its token stops working.
func TestUsers_DeleteEndsTheAccount(t *testing.T) {
	env := newTestEnv(t)
	id := addAccount(t, env, "leaving", "password123", []string{"bingo-cards"})
	user := newClient(t, env)
	signInAs(t, user, env, "leaving", "password123")
	token := createToken(t, user, env)

	env.loginAdmin(t)
	resp := env.del(t, "/api/v1/users/"+itoa(id))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()

	if name := accountName(t, getAs(t, user, env, "/api/v1/me")); name != "" {
		t.Errorf("a deleted account's session is still %q; want a guest", name)
	}
	if resp := bearerGet(t, env, "/api/v1/cards", token); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("a deleted account's token: %d; want 401", resp.StatusCode)
	}
	if resp := env.del(t, "/api/v1/users/"+itoa(id)); resp.StatusCode != http.StatusNotFound {
		t.Errorf("deleting it again: %d; want 404", resp.StatusCode)
	}
}

// TestUsers_ResetPasswordEndsSessionsAndToken: an admin's reset ends the account's
// sessions and revokes its token.
func TestUsers_ResetPasswordEndsSessionsAndToken(t *testing.T) {
	env := newTestEnv(t)
	id := addAccount(t, env, "staff", "password123", []string{"bingo-cards"})
	user := newClient(t, env)
	signInAs(t, user, env, "staff", "password123")
	token := createToken(t, user, env)

	env.loginAdmin(t)
	resp := env.patchJSON(t, "/api/v1/users/"+itoa(id), map[string]any{"password": "resetpassword"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset: %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	if name := accountName(t, getAs(t, user, env, "/api/v1/me")); name != "" {
		t.Errorf("the session from before the reset is still %q; want signed out", name)
	}
	if resp := bearerGet(t, env, "/api/v1/cards", token); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the token from before the reset: %d; want 401", resp.StatusCode)
	}
	signInAs(t, newClient(t, env), env, "staff", "resetpassword")
}

// -- Protected admin account --------------------------------------------------

func TestProtectedAdmin_CannotBeModifiedByAdmins(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	adminID := findUserID(t, env, "admin")
	path := "/api/v1/users/" + itoa(adminID)

	// PATCHing active/admin/password on the protected admin account is refused.
	for _, body := range []map[string]any{
		{"isActive": false},
		{"isAdmin": false},
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

	resp := env.patchJSON(t, "/api/v1/users/"+itoa(adminID), map[string]any{"permissions": []string{"bingo-cards"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH permissions on admin: status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

// -- Self-service password change ---------------------------------------------

func changePassword(t *testing.T, c *http.Client, e *testEnv, current, next string) *http.Response {
	t.Helper()
	return reqAs(t, c, e, http.MethodPut, "/api/v1/me/password", map[string]string{
		"currentPassword": current, "newPassword": next,
	})
}

func TestAccount_ChangePassword(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// A wrong current password is refused, and names the field.
	resp := changePassword(t, env.client, env, "nope", "password123")
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("wrong current password status = %d; want 422", resp.StatusCode)
	}
	resp.Body.Close()

	// Correct change succeeds.
	resp = changePassword(t, env.client, env, seedAdminPass, "newpassword1")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("change password status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()

	// Old password no longer works; new one does (fresh client).
	c := newClient(t, env)
	resp = postAs(t, c, env, "/api/v1/auth/login", map[string]string{"username": seedAdminUser, "password": seedAdminPass})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password login status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
	signInAs(t, c, env, seedAdminUser, "newpassword1")
}

// TestAccount_ChangePasswordEndsOtherSessions: a change signs out the account's other
// sessions and revokes its token, while this browser carries on.
func TestAccount_ChangePasswordEndsOtherSessions(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	other := newClient(t, env)
	signInAs(t, other, env, seedAdminUser, seedAdminPass)
	token := createToken(t, env.client, env)

	resp := changePassword(t, env.client, env, seedAdminPass, "newpassword1")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("change password status = %d; want 204", resp.StatusCode)
	}

	if name := env.signedInAs(t); name != seedAdminUser {
		t.Errorf("this browser after the change is %q; want still signed in", name)
	}
	if name := accountName(t, getAs(t, other, env, "/api/v1/me")); name != "" {
		t.Errorf("the other session after the change is %q; want signed out", name)
	}
	if resp := bearerGet(t, env, "/api/v1/users", token); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("the token after the change: %d; want 401", resp.StatusCode)
	}
}

// TestLoginAfterPasswordChangeStampsEpoch guards the session contract every way of
// signing in shares (auth.SignIn). A session carries the account's password epoch
// as well as its id; one minted without the epoch reads back as epoch 0, which the
// server refuses for any account whose password has ever changed. The failure is
// nasty because the sign-in itself looks fine - it answers 200 with the account -
// and only the NEXT request finds nobody. That is what a passkey sign-in once did
// for every account past epoch 0, because it stamped the id and not the epoch.
func TestLoginAfterPasswordChangeStampsEpoch(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Bump the account past epoch 0. Below this point a session missing the epoch
	// is indistinguishable from a stale one and gets rejected.
	resp := changePassword(t, env.client, env, seedAdminPass, "newpassword1")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("change password status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()

	c := newClient(t, env)
	signInAs(t, c, env, seedAdminUser, "newpassword1")

	// The assertion that matters: the session survives into the next request.
	if name := accountName(t, getAs(t, c, env, "/api/v1/me")); name != seedAdminUser {
		t.Errorf("the session a sign-in made does not resolve on the next request (who am I = %q)", name)
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
// asserting the wrong thing: GET /api/v1/game is public and varies what it shows by
// role (staff with the Game page see numbers inside the draw delay), and GET
// /api/raffles is the public raffle list, which varies its payload by role
// (raffleStaff), rather than refusing outright.
func TestPermission_EveryPageGuardsAReadRoute(t *testing.T) {
	routes := map[string]string{
		"bingo-cards":            "/api/v1/cards",
		"bingo-winners-log":      "/api/v1/winners-log",
		"bingo-patterns":         "/api/v1/patterns",
		"teahouse-announcements": "/api/announcements",
		"teahouse-affiliates":    "/api/affiliates",
		"teahouse-tea-rooms":     "/api/tea-rooms",
		"festival-map":           "/api/festival-maps",
		"festival-garapon":       "/api/garapons",
		"festival-stamp-rally":   "/api/stamp-rallies",
		"atelier-fonts":          "/api/fonts",
		"system-themes":          "/api/v1/styles",
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
	resp := getAs(t, user, env, "/api/v1/presets")
	resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		t.Errorf("GET /api/v1/presets WITH bingo-presets = %d; the grantee cannot reach their own page",
			resp.StatusCode)
	}
}

// TestUsers_ListShowsLastSignIn: the Users page sees when an account last signed in,
// and null for never.
func TestUsers_ListShowsLastSignIn(t *testing.T) {
	env := newTestEnv(t)
	addAccount(t, env, "never", "password123", nil)
	env.loginAdmin(t)

	seen := map[string]any{}
	for _, it := range decodeBody(t, env.get(t, "/api/v1/users"))["users"].([]any) {
		m, _ := it.(map[string]any)
		seen[m["username"].(string)] = m["lastLoginAt"]
	}
	if seen["never"] != nil {
		t.Errorf("an account that never signed in shows %v; want null", seen["never"])
	}
	if seen[seedAdminUser] == nil {
		t.Error("the admin just signed in; its lastLoginAt should be set")
	}
}
