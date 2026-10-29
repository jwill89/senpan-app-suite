package server_test

import (
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"app-suite/internal/auth"
	"app-suite/internal/discord"
	"app-suite/internal/discord/discordtest"
	"app-suite/internal/secretbox"
	"app-suite/internal/server"
)

// withDiscord points the test server's bot at a fake Discord, sealing tokens under the test
// session secret, and returns the fake.
func (e *testEnv) withDiscord(t *testing.T) *discordtest.Server {
	t.Helper()
	return e.withDiscordPolicy(t, false, "")
}

func (e *testEnv) withDiscordPolicy(t *testing.T, dryRun bool, redirect string) *discordtest.Server {
	t.Helper()
	f := discordtest.New()
	srv := httptest.NewServer(f.Handler())
	t.Cleanup(srv.Close)
	box, err := secretbox.New(testSecret, server.DiscordTokenPurpose)
	if err != nil {
		t.Fatal(err)
	}
	e.srv.SetDiscord(discord.NewService(srv.URL, ""), box, dryRun, redirect)
	return f
}

// discordStatus reads GET /api/discord/status as a map.
func (e *testEnv) discordStatus(t *testing.T) map[string]any {
	t.Helper()
	resp := e.get(t, "/api/discord/status")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	return decodeBody(t, resp)["status"].(map[string]any)
}

func (e *testEnv) setDiscordToken(t *testing.T, token string) map[string]any {
	t.Helper()
	resp := e.putJSON(t, "/api/discord/token", map[string]string{"token": token})
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("set token = %d: %s", resp.StatusCode, b)
	}
	return decodeBody(t, resp)["status"].(map[string]any)
}

// loginStaff creates and logs in an active non-admin account holding perms.
func (e *testEnv) loginStaff(t *testing.T, perms ...string) {
	t.Helper()
	hash, err := auth.Hash("password123")
	if err != nil {
		t.Fatal(err)
	}
	u, err := e.store.CreateUser("staff", hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.SetUserActive(u.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := e.store.SetUserPermissions(u.ID, perms); err != nil {
		t.Fatal(err)
	}
	e.postJSON(t, "/api/auth", map[string]string{"action": "login", "username": "staff", "password": "password123"}).Body.Close()
}

func TestDiscord_GuardsAdminPagesAndPickerLists(t *testing.T) {
	env := newTestEnv(t)
	env.withDiscord(t)
	for _, path := range []string{"/api/discord/status", "/api/discord/channels", "/api/discord/roles"} {
		if resp := env.get(t, path); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("anonymous %s = %d, want 401", path, resp.StatusCode)
			resp.Body.Close()
		}
	}
	// A tea-room manager can read the pickers but not the bot's page.
	env.loginStaff(t, "teahouse-tea-rooms")
	if resp := env.get(t, "/api/discord/status"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("staff status = %d, want 401 (admin only)", resp.StatusCode)
		resp.Body.Close()
	}
	if resp := env.putJSON(t, "/api/discord/token", map[string]string{"token": discordtest.Token}); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("staff set token = %d, want 401", resp.StatusCode)
		resp.Body.Close()
	}
	for _, path := range []string{"/api/discord/channels", "/api/discord/roles"} {
		resp := env.get(t, path)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("staff %s = %d, want 200", path, resp.StatusCode)
		}
	}
}

func TestDiscord_PickerListsNeedAPostingPermission(t *testing.T) {
	env := newTestEnv(t)
	env.withDiscord(t)
	env.loginStaff(t, "bingo-cards")
	if resp := env.get(t, "/api/discord/channels"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("bingo-only staff channels = %d, want 403", resp.StatusCode)
		resp.Body.Close()
	}
}

func TestDiscord_StatusBeforeAToken(t *testing.T) {
	env := newTestEnv(t)
	env.withDiscord(t)
	env.loginAdmin(t)
	st := env.discordStatus(t)
	if st["key_available"] != true || st["token_set"] != false || st["connected"] != false {
		t.Fatalf("status = %v", st)
	}
	if !strings.Contains(st["problem"].(string), "Paste the bot token") {
		t.Fatalf("problem = %q", st["problem"])
	}
	invite := st["invite_url"].(string)
	for _, want := range []string{"client_id=1563399760994635906", "guild_id=999011226568888410", "permissions=8", "scope=bot"} {
		if !strings.Contains(invite, want) {
			t.Errorf("invite %q lacks %s", invite, want)
		}
	}
	if st["guild_id"] != "999011226568888410" || st["public_base_url"] != "https://apps.senpan.cafe" {
		t.Fatalf("defaults = %v / %v", st["guild_id"], st["public_base_url"])
	}
}

func TestDiscord_SetTokenStoresItSealedAndConnects(t *testing.T) {
	env := newTestEnv(t)
	env.withDiscord(t)
	env.loginAdmin(t)
	st := env.setDiscordToken(t, "Bot "+discordtest.Token) // a pasted "Bot " prefix is stripped
	if st["connected"] != true || st["in_guild"] != true || st["bot_is_admin"] != true || st["problem"] != "" {
		t.Fatalf("status after token = %v", st)
	}
	if st["bot_name"] != "Senpan Tea House" || st["guild_name"] != "Senpan Tea House" {
		t.Fatalf("identity = %v / %v", st["bot_name"], st["guild_name"])
	}
	stored, err := env.store.GetSetting("discord_bot_token")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, "v1:") || strings.Contains(stored, discordtest.Token) {
		t.Fatalf("the token is not sealed in the database: %q", stored)
	}
	// The token never comes back out.
	resp := env.get(t, "/api/discord/status")
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(raw), discordtest.Token) {
		t.Fatal("the status response carries the token")
	}
	resp = env.get(t, "/api/settings")
	raw, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(raw), "discord_bot_token") || strings.Contains(string(raw), "v1:") {
		t.Fatal("the token setting leaks through /api/settings")
	}
}

func TestDiscord_RefusedTokenIsNotStored(t *testing.T) {
	env := newTestEnv(t)
	env.withDiscord(t)
	env.loginAdmin(t)
	resp := env.putJSON(t, "/api/discord/token", map[string]string{"token": "not-the-token"})
	body := decodeBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body["error"].(string), "refused") {
		t.Fatalf("wrong token = %d %v", resp.StatusCode, body)
	}
	if v, _ := env.store.GetSetting("discord_bot_token"); v != "" {
		t.Fatal("a refused token was stored")
	}
	for _, bad := range []string{"", "two words", strings.Repeat("x", 201)} {
		resp := env.putJSON(t, "/api/discord/token", map[string]string{"token": bad})
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("token %q = %d, want 400", bad, resp.StatusCode)
		}
	}
}

func TestDiscord_TokenFromAnotherApplicationIsRefused(t *testing.T) {
	env := newTestEnv(t)
	f := env.withDiscord(t)
	f.Lock()
	f.AppID = "1111111111111111111"
	f.AppName = "Some Other Bot"
	f.Unlock()
	env.loginAdmin(t)
	resp := env.putJSON(t, "/api/discord/token", map[string]string{"token": discordtest.Token})
	body := decodeBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body["error"].(string), "Some Other Bot") {
		t.Fatalf("other application's token = %d %v", resp.StatusCode, body)
	}
}

func TestDiscord_WithoutASessionSecretATokenCantBeStored(t *testing.T) {
	env := newTestEnv(t)
	f := discordtest.New()
	fake := httptest.NewServer(f.Handler())
	defer fake.Close()
	env.srv.SetDiscord(discord.NewService(fake.URL, ""), nil, false, "")
	env.loginAdmin(t)
	resp := env.putJSON(t, "/api/discord/token", map[string]string{"token": discordtest.Token})
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("set token without a key = %d, want 409", resp.StatusCode)
	}
	if st := env.discordStatus(t); st["key_available"] != false || !strings.Contains(st["problem"].(string), "session secret") {
		t.Fatalf("status = %v", st)
	}
}

func TestDiscord_TokenSealedUnderAnotherSecretIsUnreadable(t *testing.T) {
	env := newTestEnv(t)
	other, _ := secretbox.New("a different servers secret", server.DiscordTokenPurpose)
	sealed, _ := other.Seal(discordtest.Token)
	if err := env.store.SetSetting("discord_bot_token", sealed); err != nil {
		t.Fatal(err)
	}
	env.withDiscord(t) // applies the saved configuration
	env.loginAdmin(t)
	st := env.discordStatus(t)
	if st["token_set"] != true || st["token_readable"] != false || st["connected"] != false {
		t.Fatalf("status = %v", st)
	}
	if !strings.Contains(st["problem"].(string), "can't be read") {
		t.Fatalf("problem = %q", st["problem"])
	}
}

func TestDiscord_ClearTokenTakesTheBotOff(t *testing.T) {
	env := newTestEnv(t)
	env.withDiscord(t)
	env.loginAdmin(t)
	env.setDiscordToken(t, discordtest.Token)
	resp := env.del(t, "/api/discord/token")
	st := decodeBody(t, resp)["status"].(map[string]any)
	if st["token_set"] != false || st["connected"] != false {
		t.Fatalf("status after removal = %v", st)
	}
}

func TestDiscord_ConfigIsValidatedAndSaved(t *testing.T) {
	env := newTestEnv(t)
	env.withDiscord(t)
	env.loginAdmin(t)
	valid := map[string]any{
		"guild_id": "999011226568888410", "application_id": "1563399760994635906",
		"status_text": "Pouring tea", "public_base_url": "https://apps.senpan.cafe/",
		"test_mode": false, "test_channel_id": "",
	}
	with := func(k string, v any) map[string]any {
		out := maps.Clone(valid)
		out[k] = v
		return out
	}
	for name, body := range map[string]map[string]any{
		"guild not digits":         with("guild_id", "senpan"),
		"application blank":        with("application_id", ""),
		"status too long":          with("status_text", strings.Repeat("a", 129)),
		"base url not absolute":    with("public_base_url", "apps.senpan.cafe"),
		"test mode without a room": with("test_mode", true),
		"test channel not digits":  with("test_channel_id", "#general"),
	} {
		resp := env.putJSON(t, "/api/discord/config", body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, resp.StatusCode)
		}
	}
	saved := with("test_mode", true)
	saved["test_channel_id"] = "200000000000000005"
	resp := env.putJSON(t, "/api/discord/config", saved)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid config = %d", resp.StatusCode)
	}
	st := decodeBody(t, resp)["status"].(map[string]any)
	if st["test_mode"] != true || st["test_channel_id"] != "200000000000000005" || st["status_text"] != "Pouring tea" {
		t.Fatalf("saved = %v", st)
	}
	if st["public_base_url"] != "https://apps.senpan.cafe" {
		t.Fatalf("trailing slash kept: %v", st["public_base_url"])
	}
}

func TestDiscord_TestMessage(t *testing.T) {
	env := newTestEnv(t)
	f := env.withDiscord(t)
	env.loginAdmin(t)

	// Not connected yet.
	resp := env.postJSON(t, "/api/discord/test", map[string]string{"channel_id": "200000000000000005"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("test without a token = %d, want 409", resp.StatusCode)
	}

	env.setDiscordToken(t, discordtest.Token)
	resp = env.postJSON(t, "/api/discord/test", map[string]string{"channel_id": "200000000000000005"})
	body := decodeBody(t, resp)
	if resp.StatusCode != http.StatusOK || body["sent"] != true || body["message_id"] == "" {
		t.Fatalf("test = %d %v", resp.StatusCode, body)
	}
	msgs := f.Messages()
	if len(msgs) != 1 || msgs[0].ChannelID != "200000000000000005" || !strings.Contains(msgs[0].Content, "Test message") {
		t.Fatalf("messages = %v", msgs)
	}
	if len(msgs[0].AllowedMentions.Parse) != 0 || len(msgs[0].AllowedMentions.Roles) != 0 {
		t.Fatalf("a test message may ping no one: %+v", msgs[0].AllowedMentions)
	}

	// A channel Discord doesn't know is a named error, not a 500.
	resp = env.postJSON(t, "/api/discord/test", map[string]string{"channel_id": "209999999999999999"})
	body = decodeBody(t, resp)
	if resp.StatusCode != http.StatusBadGateway || !strings.Contains(body["error"].(string), "couldn't find") {
		t.Fatalf("unknown channel = %d %v", resp.StatusCode, body)
	}
}

func TestDiscord_TestMessageHonorsTheDevValves(t *testing.T) {
	env := newTestEnv(t)
	f := env.withDiscordPolicy(t, true, "")
	env.loginAdmin(t)
	env.setDiscordToken(t, discordtest.Token)
	resp := env.postJSON(t, "/api/discord/test", map[string]string{"channel_id": "200000000000000005"})
	body := decodeBody(t, resp)
	if body["suppressed"] != true || body["sent"] != false || len(f.Messages()) != 0 {
		t.Fatalf("dry run sent something: %v, %v", body, f.Messages())
	}

	env2 := newTestEnv(t)
	f2 := env2.withDiscordPolicy(t, false, "200000000000000004")
	env2.loginAdmin(t)
	env2.setDiscordToken(t, discordtest.Token)
	resp = env2.postJSON(t, "/api/discord/test", map[string]string{"channel_id": "200000000000000005"})
	resp.Body.Close()
	if msgs := f2.Messages(); len(msgs) != 1 || msgs[0].ChannelID != "200000000000000004" {
		t.Fatalf("redirect not applied: %v", msgs)
	}
}

func TestDiscord_ChannelsAndRolesForThePickers(t *testing.T) {
	env := newTestEnv(t)
	env.withDiscord(t)
	env.loginAdmin(t)

	// Before a token: an answer the pickers can fall back on, not an error.
	body := decodeBody(t, env.get(t, "/api/discord/channels"))
	if body["available"] != false || body["problem"] == "" || len(body["channels"].([]any)) != 0 {
		t.Fatalf("channels without a token = %v", body)
	}

	env.setDiscordToken(t, discordtest.Token)
	body = decodeBody(t, env.get(t, "/api/discord/channels"))
	if body["available"] != true {
		t.Fatalf("channels = %v", body)
	}
	var got []string
	for _, c := range body["channels"].([]any) {
		m := c.(map[string]any)
		got = append(got, m["name"].(string)+"|"+m["type"].(string)+"|"+m["category"].(string))
	}
	// Uncategorized first, then the category's channels in position order; no voice channel,
	// no category itself.
	want := []string{"test-channel|text|", "announcements|announcement|Tea House", "tea-rooms|text|Tea House", "book-club|text|Tea House"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("channels = %v\nwant       %v", got, want)
	}

	body = decodeBody(t, env.get(t, "/api/discord/roles"))
	var roles []string
	for _, r := range body["roles"].([]any) {
		m := r.(map[string]any)
		roles = append(roles, m["name"].(string)+"|"+m["color"].(string))
	}
	want = []string{"Senpan Bot|", "Staff|#e53170", "Event Pings|#3498db"}
	if strings.Join(roles, ",") != strings.Join(want, ",") {
		t.Fatalf("roles = %v\nwant    %v", roles, want)
	}
}

func TestDiscord_ConfigChangesRefreshOtherAdmins(t *testing.T) {
	for path, want := range map[string]string{
		"/api/discord/config": "discord",
		"/api/discord/token":  "discord",
		"/api/discord/test":   "",
	} {
		got, ok := server.AdminMutationResourceForTest(path)
		if got != want || ok != (want != "") {
			t.Errorf("%s -> %q, %v; want %q", path, got, ok, want)
		}
	}
}
