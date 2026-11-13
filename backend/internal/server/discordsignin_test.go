package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"golang.org/x/oauth2"
)

// standIn plays Discord: its consent page issues a code bound to the sign-in's PKCE
// challenge, its return address and the account consenting, and its token endpoint
// redeems a code once, for that address and the verifier that hashes to the challenge.
type standIn struct {
	*httptest.Server
	mu     sync.Mutex
	issued int
	codes  map[string]grant  // code to what it was issued for
	tokens map[string]string // access token to the account's JSON
}

type grant struct{ challenge, redirect, account string }

// Discord accounts the tests sign in as.
const (
	lune  = `{"id":"80351110224678912","username":"lune","global_name":"Lune"}`
	mirai = `{"id":"90351110224678913","username":"mirai","global_name":null}`
	// adminish is a Discord account whose username is the reserved admin's.
	adminish = `{"id":"70351110224678914","username":"admin","global_name":"Not The Admin"}`
)

func newStandIn(t *testing.T, e *testEnv) *standIn {
	t.Helper()
	s := &standIn{codes: map[string]grant{}, tokens: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		g, ok := s.codes[r.FormValue("code")]
		w.Header().Set("Content-Type", "application/json")
		if !ok || r.FormValue("client_id") != "senpan-test" || r.FormValue("client_secret") != "not-a-secret" ||
			r.FormValue("grant_type") != "authorization_code" ||
			oauth2.S256ChallengeFromVerifier(r.FormValue("code_verifier")) != g.challenge ||
			r.FormValue("redirect_uri") != g.redirect {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		delete(s.codes, r.FormValue("code"))
		token := "granted-" + r.FormValue("code")
		s.tokens[token] = g.account
		_, _ = w.Write([]byte(`{"access_token":"` + token + `","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("GET /me", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		account, ok := s.tokens[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(account))
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	e.srv.RouteDiscordSignInForTest(s.URL)
	return s
}

// consent is account approving the sign-in on the consent page at location: the
// callback address Discord sends the browser back to, with its code and the state.
func (s *standIn) consent(t *testing.T, location, account string) string {
	t.Helper()
	to, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	if to.Scheme+"://"+to.Host+to.Path != s.URL+"/authorize" {
		t.Fatalf("sent to %s; want the consent page", location)
	}
	q := to.Query()
	if q.Get("scope") != "identify" || q.Get("code_challenge_method") != "S256" || len(q.Get("code_challenge")) != 43 ||
		q.Get("client_id") != "senpan-test" || q.Get("state") == "" {
		t.Fatalf("consent page asks for the wrong thing: %s", location)
	}
	s.mu.Lock()
	s.issued++
	code := "code-" + strconv.Itoa(s.issued)
	s.codes[code] = grant{challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri"), account: account}
	s.mu.Unlock()
	back := url.Values{"code": {code}, "state": {q.Get("state")}}
	return q.Get("redirect_uri") + "?" + back.Encode()
}

// browser is a client of its own that, like a browser's address bar, shows each
// redirect rather than following it.
func browser(t *testing.T, e *testEnv) *http.Client {
	t.Helper()
	c := newClient(t, e)
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

// open is c opening address (a full URL, or a path on the test server), and where it
// is sent next.
func open(t *testing.T, c *http.Client, e *testEnv, address string) string {
	t.Helper()
	if strings.HasPrefix(address, "/") {
		address = e.url(address)
	}
	resp, err := c.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("GET %s: %d; want 302", address, resp.StatusCode)
	}
	return resp.Header.Get("Location")
}

// signInWithDiscord walks c from start (a path or link) through Discord's consent, as
// account, and answers where the callback sends it.
func (s *standIn) signIn(t *testing.T, c *http.Client, e *testEnv, start, account string) string {
	t.Helper()
	return open(t, c, e, s.consent(t, open(t, c, e, start), account))
}

// discordInvite is a link the admin on e.client makes: to sign up, or, with userID,
// to link that account.
func discordInvite(t *testing.T, e *testEnv, userID int64) string {
	t.Helper()
	body := map[string]any{}
	if userID != 0 {
		body["userId"] = userID
	}
	resp := e.postJSON(t, "/api/v1/discord-invites", body)
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("make a Discord link: %d", resp.StatusCode)
	}
	link, _ := decodeBody(t, resp)["url"].(string)
	if !strings.HasPrefix(link, e.url("/api/v1/auth/discord/invite/")) {
		t.Fatalf("link %q is not this site's", link)
	}
	return link
}

// me is who GET /me says c is: the account's fields, nil for nobody.
func me(t *testing.T, c *http.Client, e *testEnv) map[string]any {
	t.Helper()
	account, _ := decodeBody(t, getAs(t, c, e, "/api/v1/me"))["account"].(map[string]any)
	return account
}

// userRow is the Users page's row for username.
func userRow(t *testing.T, e *testEnv, username string) map[string]any {
	t.Helper()
	data := decodeBody(t, e.get(t, "/api/v1/users"))
	list, _ := data["users"].([]any)
	for _, it := range list {
		if m, _ := it.(map[string]any); m["username"] == username {
			return m
		}
	}
	t.Fatalf("no account %q on the Users page", username)
	return nil
}

// Without the application's credentials Discord sign-in is off: the config says so,
// the start lands on the sign-in page, and an admin can't make a link.
func TestDiscordSignInOffWithoutCredentials(t *testing.T) {
	e := newTestEnv(t)
	if on, _ := decodeBody(t, e.get(t, "/api/v1/config"))["discordSignIn"].(bool); on {
		t.Error("config says Discord sign-in is on")
	}
	c := browser(t, e)
	if got := open(t, c, e, "/api/v1/auth/discord/start"); got != "/admin/login?error=discord_unavailable" {
		t.Errorf("start sent to %q", got)
	}
	e.loginAdmin(t)
	resp := e.postJSON(t, "/api/v1/discord-invites", map[string]any{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("make a link: %d; want 503", resp.StatusCode)
	}
}

// Nobody signs up by signing in: a Discord account no account is linked to is sent
// back to the sign-in page, and so are a declined consent and a forged state.
func TestDiscordSignInRefusals(t *testing.T) {
	e := newTestEnv(t)
	d := newStandIn(t, e)
	if on, _ := decodeBody(t, e.get(t, "/api/v1/config"))["discordSignIn"].(bool); !on {
		t.Error("config says Discord sign-in is off")
	}
	c := browser(t, e)
	if got := d.signIn(t, c, e, "/api/v1/auth/discord/start", lune); got != "/admin/login?error=discord_not_linked" {
		t.Errorf("unlinked Discord account sent to %q", got)
	}
	if me(t, c, e) != nil {
		t.Error("an unlinked Discord account signed in")
	}

	// Declined at Discord.
	consent := open(t, c, e, "/api/v1/auth/discord/start")
	state, _ := url.Parse(consent)
	declined := e.url("/api/v1/auth/discord/callback?error=access_denied&state=" + url.QueryEscape(state.Query().Get("state")))
	if got := open(t, c, e, declined); got != "/admin/login?error=discord_denied" {
		t.Errorf("declined sent to %q", got)
	}

	// A state that isn't the session's, and a replayed callback.
	back := d.consent(t, open(t, c, e, "/api/v1/auth/discord/start"), lune)
	forged := strings.Replace(back, "state=", "state=x", 1)
	if got := open(t, c, e, forged); got != "/admin/login?error=discord_failed" {
		t.Errorf("forged state sent to %q", got)
	}
	if got := open(t, c, e, back); got != "/admin/login?error=discord_failed" {
		t.Errorf("callback after its state was used sent to %q", got)
	}
	// A callback in another browser than the one that started it.
	back = d.consent(t, open(t, c, e, "/api/v1/auth/discord/start"), lune)
	if got := open(t, browser(t, e), e, back); got != "/admin/login?error=discord_failed" {
		t.Errorf("callback in another browser sent to %q", got)
	}
}

// An admin's sign-up link makes an active account named after the Discord account,
// with no permissions and no password, signed in at once; it works once. The account
// then signs in with Discord, landing on the page the start named.
func TestDiscordSignUpLink(t *testing.T) {
	e := newTestEnv(t)
	d := newStandIn(t, e)
	e.loginAdmin(t)
	link := discordInvite(t, e, 0)

	c := browser(t, e)
	if got := d.signIn(t, c, e, link, lune); got != "/admin" {
		t.Fatalf("sign-up sent to %q; want /admin", got)
	}
	account := me(t, c, e)
	if account["username"] != "lune" || account["isAdmin"] != false || account["hasPassword"] != false ||
		len(account["permissions"].([]any)) != 0 {
		t.Errorf("signed up as %v", account)
	}
	row := userRow(t, e, "lune")
	discord, _ := row["discord"].(map[string]any)
	if row["isActive"] != true || discord["username"] != "lune" || discord["displayName"] != "Lune" {
		t.Errorf("Users page shows %v", row)
	}
	if _, kept := discord["id"]; kept {
		t.Error("the Users page carries the Discord id")
	}

	// Used once.
	if got := open(t, browser(t, e), e, link); got != "/admin/login?error=discord_link_expired" {
		t.Errorf("second use sent to %q", got)
	}
	if got := open(t, browser(t, e), e, "/api/v1/auth/discord/invite/not-a-link"); got != "/admin/login?error=discord_link_expired" {
		t.Errorf("unknown link sent to %q", got)
	}

	// Signing in again, in a new browser, lands where the start said; another host
	// is never a place to land.
	c = browser(t, e)
	if got := d.signIn(t, c, e, "/api/v1/auth/discord/start?next=/admin/bingo/cards", lune); got != "/admin/bingo/cards" {
		t.Errorf("sign-in sent to %q", got)
	}
	if name, _ := me(t, c, e)["username"].(string); name != "lune" {
		t.Errorf("signed in as %q", name)
	}
	for _, next := range []string{"//evil.example", "https://evil.example", "/\\evil.example"} {
		got := d.signIn(t, browser(t, e), e, "/api/v1/auth/discord/start?next="+url.QueryEscape(next), lune)
		if got != "/admin" {
			t.Errorf("next %q sent to %q", next, got)
		}
	}

	// It has no password to sign in with, and the form answers as for a wrong one.
	resp := postAs(t, browser(t, e), e, "/api/v1/auth/login", map[string]string{"username": "lune", "password": ""})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("password sign-in with none: %d; want 401", resp.StatusCode)
	}

	// A Discord account already linked can't sign up again, and the link survives.
	again := discordInvite(t, e, 0)
	if got := d.signIn(t, browser(t, e), e, again, lune); got != "/admin/login?error=discord_taken" {
		t.Errorf("linked Discord account signing up sent to %q", got)
	}
	if got := d.signIn(t, browser(t, e), e, again, mirai); got != "/admin" {
		t.Errorf("the link refused for a taken account then sent to %q", got)
	}
}

// A sign-up takes the next free name when the Discord username is taken or is the
// admin account's.
func TestDiscordSignUpNames(t *testing.T) {
	e := newTestEnv(t)
	d := newStandIn(t, e)
	e.loginAdmin(t)
	addAccount(t, e, "lune", "password-1", nil)
	c := browser(t, e)
	d.signIn(t, c, e, discordInvite(t, e, 0), lune)
	if name, _ := me(t, c, e)["username"].(string); name != "lune-2" {
		t.Errorf("signed up as %q; want lune-2", name)
	}
	c = browser(t, e)
	d.signIn(t, c, e, discordInvite(t, e, 0), adminish)
	if name, _ := me(t, c, e)["username"].(string); name != "admin-2" {
		t.Errorf("signed up as %q; want admin-2", name)
	}
}

// An account with no password sets one without a current password, adds a passkey
// without one, and can't lose its last way in.
func TestDiscordAccountWithoutPassword(t *testing.T) {
	e := newTestEnv(t)
	d := newStandIn(t, e)
	e.loginAdmin(t)
	c := browser(t, e)
	d.signIn(t, c, e, discordInvite(t, e, 0), lune)
	id := findUserID(t, e, "lune")

	// WebAuthn needs a named host (passkeyDo), which sends as e.client: it lends the
	// account's cookies for the one request.
	adminJar := e.client.Jar
	e.client.Jar = c.Jar
	resp := passkeyDo(t, e, http.MethodPost, "/api/v1/me/passkeys/begin", map[string]string{})
	e.client.Jar = adminJar
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("begin a passkey with no password: %d; want 200", resp.StatusCode)
	}

	// Discord is its only way in.
	resp = e.del(t, "/api/v1/users/"+strconv.FormatInt(id, 10)+"/discord")
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("unlink the only way in: %d; want 409", resp.StatusCode)
	}
	if userRow(t, e, "lune")["discord"] == nil {
		t.Error("a refused unlink unlinked")
	}

	resp = reqAs(t, c, e, http.MethodPut, "/api/v1/me/password", map[string]string{"newPassword": "a-new-password"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("set a first password: %d; want 204", resp.StatusCode)
	}
	if me(t, c, e)["hasPassword"] != true {
		t.Error("still no password")
	}
	resp = reqAs(t, c, e, http.MethodPut, "/api/v1/me/password", map[string]string{"currentPassword": "wrong", "newPassword": "another-password"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("change it without the current one: %d; want 422", resp.StatusCode)
	}

	resp = e.del(t, "/api/v1/users/"+strconv.FormatInt(id, 10)+"/discord")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unlink: %d; want 204", resp.StatusCode)
	}
	if userRow(t, e, "lune")["discord"] != nil {
		t.Error("still linked")
	}
	resp = e.del(t, "/api/v1/users/"+strconv.FormatInt(id, 10)+"/discord")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unlink again: %d; want 404", resp.StatusCode)
	}
	if got := d.signIn(t, browser(t, e), e, "/api/v1/auth/discord/start", lune); got != "/admin/login?error=discord_not_linked" {
		t.Errorf("unlinked Discord account sent to %q", got)
	}
}

// A link made for an account links whoever opens it, with their Discord account, and
// signs them in to it; a new link replaces the last. A Discord account linked to
// another account is refused and the link left unused; an inactive account is linked
// but not signed in.
func TestDiscordLinkToAccount(t *testing.T) {
	e := newTestEnv(t)
	d := newStandIn(t, e)
	e.loginAdmin(t)
	id := addAccount(t, e, "kiri", "password-1", []string{"bingo-cards"})

	replaced := discordInvite(t, e, id)
	link := discordInvite(t, e, id)
	if got := open(t, browser(t, e), e, replaced); got != "/admin/login?error=discord_link_expired" {
		t.Errorf("replaced link sent to %q", got)
	}

	// lune is linked to an account of its own first.
	d.signIn(t, browser(t, e), e, discordInvite(t, e, 0), lune)
	if got := d.signIn(t, browser(t, e), e, link, lune); got != "/admin/login?error=discord_taken" {
		t.Errorf("taken Discord account sent to %q", got)
	}

	c := browser(t, e)
	if got := d.signIn(t, c, e, link, mirai); got != "/admin" {
		t.Fatalf("link sent to %q", got)
	}
	if name, _ := me(t, c, e)["username"].(string); name != "kiri" {
		t.Errorf("signed in as %q; want kiri", name)
	}
	discord, _ := userRow(t, e, "kiri")["discord"].(map[string]any)
	if discord["username"] != "mirai" || discord["displayName"] != "" {
		t.Errorf("kiri's Discord account is %v", discord)
	}
	c = browser(t, e)
	if got := d.signIn(t, c, e, "/api/v1/auth/discord/start", mirai); got != "/admin" {
		t.Errorf("sign-in sent to %q", got)
	}

	// Renamed, it keeps its Discord account.
	resp := e.patchJSON(t, "/api/v1/users/"+strconv.FormatInt(id, 10), map[string]any{"username": "Kiri Tane"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rename: %d", resp.StatusCode)
	}
	if name, _ := me(t, c, e)["username"].(string); name != "Kiri Tane" {
		t.Errorf("signed in as %q after the rename", name)
	}

	// An inactive account is linked, and not signed in.
	other := addAccount(t, e, "sora", "password-2", nil)
	resp = e.patchJSON(t, "/api/v1/users/"+strconv.FormatInt(other, 10), map[string]any{"isActive": false})
	resp.Body.Close()
	c = browser(t, e)
	if got := d.signIn(t, c, e, discordInvite(t, e, other), adminish); got != "/admin/login?error=discord_inactive" {
		t.Errorf("link to an inactive account sent to %q", got)
	}
	if me(t, c, e) != nil {
		t.Error("an inactive account signed in")
	}
	if userRow(t, e, "sora")["discord"] == nil {
		t.Error("the inactive account wasn't linked")
	}
	if got := d.signIn(t, browser(t, e), e, "/api/v1/auth/discord/start", adminish); got != "/admin/login?error=discord_inactive" {
		t.Errorf("inactive account's sign-in sent to %q", got)
	}
}

// Only the admin account makes a link to itself or unlinks its own Discord account;
// another admin may not, nor may anyone but an admin, nor an access token.
func TestDiscordLinkGuards(t *testing.T) {
	e := newTestEnv(t)
	d := newStandIn(t, e)
	adminID := findUserIDAsAdmin(t, e)

	other := newClient(t, e)
	otherID := addAccount(t, e, "second-admin", "password-2", nil)
	resp := e.patchJSON(t, "/api/v1/users/"+strconv.FormatInt(otherID, 10), map[string]any{"isAdmin": true})
	resp.Body.Close()
	signInAs(t, other, e, "second-admin", "password-2")
	resp = postAs(t, other, e, "/api/v1/discord-invites", map[string]any{"userId": adminID})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("another admin's link to the admin account: %d; want 403", resp.StatusCode)
	}
	resp = postAs(t, other, e, "/api/v1/discord-invites", map[string]any{"userId": 999999})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("link to no account: %d; want 404", resp.StatusCode)
	}

	// The admin account links itself.
	c := browser(t, e)
	if got := d.signIn(t, c, e, discordInvite(t, e, adminID), lune); got != "/admin" {
		t.Errorf("admin's own link sent to %q", got)
	}
	resp = reqAs(t, other, e, http.MethodDelete, "/api/v1/users/"+strconv.FormatInt(adminID, 10)+"/discord", nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("another admin unlinking the admin account: %d; want 403", resp.StatusCode)
	}

	// Not an admin.
	plain := newClient(t, e)
	addAccount(t, e, "plain", "password-3", []string{"bingo-cards"})
	signInAs(t, plain, e, "plain", "password-3")
	resp = postAs(t, plain, e, "/api/v1/discord-invites", map[string]any{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a non-admin's link: %d; want 403", resp.StatusCode)
	}

	// An access token.
	tokenResp := postAs(t, other, e, "/api/v1/me/token", nil)
	token, _ := decodeBody(t, tokenResp)["token"].(string)
	req, _ := http.NewRequest(http.MethodPost, e.url("/api/v1/discord-invites"), strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Transport: e.client.Transport}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a token's link: %d; want 403", resp.StatusCode)
	}
}

// A username another account has, or the admin account's, is refused; the admin
// account isn't renamed.
func TestRenameAccount(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	id := addAccount(t, e, "kiri", "password-1", nil)
	addAccount(t, e, "sora", "password-2", nil)
	path := "/api/v1/users/" + strconv.FormatInt(id, 10)
	for name, want := range map[string]int{"sora": http.StatusConflict, "ADMIN": http.StatusConflict, "   ": http.StatusUnprocessableEntity} {
		resp := e.patchJSON(t, path, map[string]any{"username": name})
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("rename to %q: %d; want %d", name, resp.StatusCode, want)
		}
	}
	resp := e.patchJSON(t, path, map[string]any{"username": "  Kiri  "})
	if got, _ := decodeBody(t, resp)["username"].(string); got != "Kiri" {
		t.Errorf("renamed to %q; want Kiri", got)
	}
	signInAs(t, newClient(t, e), e, "Kiri", "password-1")
	resp = e.patchJSON(t, "/api/v1/users/"+strconv.FormatInt(findUserIDAsAdmin(t, e), 10), map[string]any{"username": "root"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("rename the admin account: %d; want 403", resp.StatusCode)
	}
}

// findUserIDAsAdmin signs e.client in as the admin account and answers its id.
func findUserIDAsAdmin(t *testing.T, e *testEnv) int64 {
	t.Helper()
	e.loginAdmin(t)
	var account struct {
		Account struct {
			ID int64 `json:"id"`
		} `json:"account"`
	}
	resp := e.get(t, "/api/v1/me")
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		t.Fatal(err)
	}
	return account.Account.ID
}
