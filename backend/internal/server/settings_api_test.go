package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// decodeInto reads a JSON body into v, failing the test on a bad status or body.
func decodeInto(t *testing.T, resp *http.Response, want int, v any) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d, want %d: %s", resp.StatusCode, want, body)
	}
	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			t.Fatal(err)
		}
	}
}

// siteSettings reads the site's settings as an editable map.
func siteSettings(t *testing.T, env *testEnv) map[string]any {
	t.Helper()
	var s map[string]any
	decodeInto(t, env.get(t, "/api/v1/settings"), http.StatusOK, &s)
	delete(s, "uploadedFonts")
	return s
}

// saveSettings reads the settings, applies changes, and saves them all back.
func saveSettings(t *testing.T, env *testEnv, changes map[string]any) *http.Response {
	t.Helper()
	s := siteSettings(t, env)
	for k, v := range changes {
		s[k] = v
	}
	return env.putJSON(t, "/api/v1/settings", s)
}

// GET /api/v1/settings is public and typed, with the defaults until a save.
func TestSettingsReadIsPublicWithDefaults(t *testing.T) {
	env := newTestEnv(t)
	s := siteSettings(t, env)
	if s["appTitle"] != "Senpan App Suite" || s["defaultDrawDelay"] != float64(0) || s["hideBingo"] != false {
		t.Errorf("defaults: %v", s)
	}
	if _, ok := s["googleFontsApiKey"]; ok {
		t.Error("the public settings carry the Google Fonts key")
	}
}

// A save replaces the settings, a value out of range is a 422 naming the field, and
// a save made from an older copy is a 409.
func TestSettingsSaveValidatesAndRefusesAStaleCopy(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	var saved map[string]any
	decodeInto(t, saveSettings(t, env, map[string]any{"appTitle": "Tea House", "customCardCost": 5000000, "hideBingo": true}), http.StatusOK, &saved)
	if saved["appTitle"] != "Tea House" || saved["customCardCost"] != float64(5000000) || saved["hideBingo"] != true || saved["updatedAt"] == "" {
		t.Fatalf("after the save: %v", saved)
	}

	for field, bad := range map[string]any{
		"defaultDrawDelay": 61, "customCardCost": -1, "frequentWinnerHours": 0,
		"headerFont": "Evil'}body{", "aniListApiUrl": "https://evil.example.com",
	} {
		resp := saveSettings(t, env, map[string]any{field: bad})
		_, status, detail := problem(t, resp)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("%s = %v: status %d (%s), want 422", field, bad, status, detail)
		}
	}

	stale := siteSettings(t, env)
	decodeInto(t, saveSettings(t, env, map[string]any{"appTitle": "Newer"}), http.StatusOK, nil)
	stale["appTitle"] = "From an old page"
	resp := env.putJSON(t, "/api/v1/settings", stale)
	if _, status, _ := problem(t, resp); status != http.StatusConflict {
		t.Fatalf("a save from an older copy: %d, want 409", status)
	}
	if s := siteSettings(t, env); s["appTitle"] != "Newer" {
		t.Errorf("the stale save landed: %v", s["appTitle"])
	}
}

// Saving the settings needs the system-settings permission; the integrations need an
// admin, so a grantee of the settings page never sees or overwrites a secret.
func TestSettingsAndIntegrationsAccess(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	const secret = "fonts-api-key-do-not-clobber"
	decodeInto(t, env.putJSON(t, "/api/v1/settings/integrations", map[string]any{
		"googleFontsApiKey": secret,
		"bookClubWebhooks":  map[string]string{"yaoi": "https://discord.com/api/webhooks/1/abc"},
	}), http.StatusOK, nil)
	s := siteSettings(t, env)
	s["appTitle"] = "Renamed By Staff"

	// The test server's clients share one cookie jar slot, so one account at a time.
	staff := makeActiveUser(t, env, "settingsstaff", "correct horse battery", []string{"system-settings"})
	decodeInto(t, reqAs(t, staff, env, http.MethodPut, "/api/v1/settings", s), http.StatusOK, nil)
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		resp := reqAs(t, staff, env, method, "/api/v1/settings/integrations", map[string]any{"googleFontsApiKey": "", "bookClubWebhooks": map[string]string{}})
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s integrations as a settings grantee: %d, want 403", method, resp.StatusCode)
		}
		resp.Body.Close()
	}

	env.loginAdmin(t)
	var integrations map[string]any
	decodeInto(t, env.get(t, "/api/v1/settings/integrations"), http.StatusOK, &integrations)
	if integrations["googleFontsApiKey"] != secret {
		t.Errorf("the secret after a grantee's save: %v", integrations["googleFontsApiKey"])
	}
	if s := siteSettings(t, env); s["appTitle"] != "Renamed By Staff" {
		t.Errorf("the grantee's own edit: %v", s["appTitle"])
	}

	stranger := makeActiveUser(t, env, "stranger", "correct horse battery", []string{"bingo-cards"})
	if resp := reqAs(t, stranger, env, http.MethodPut, "/api/v1/settings", siteSettings(t, env)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a save without the permission: %d, want 403", resp.StatusCode)
		resp.Body.Close()
	}
	if resp := reqAs(t, stranger, env, http.MethodPost, "/api/v1/styles", map[string]any{}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a theme without the permission: %d, want 403", resp.StatusCode)
		resp.Body.Close()
	}
}

// An admin can clear a secret on purpose, a webhook must be Discord's, and a club
// the registry doesn't have is refused.
func TestIntegrationsSave(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	decodeInto(t, env.putJSON(t, "/api/v1/settings/integrations", map[string]any{
		"googleFontsApiKey": "key", "bookClubWebhooks": map[string]string{"yaoi": "https://discord.com/api/webhooks/1/abc"},
	}), http.StatusOK, nil)

	var cleared map[string]any
	decodeInto(t, env.putJSON(t, "/api/v1/settings/integrations", map[string]any{
		"googleFontsApiKey": "", "bookClubWebhooks": map[string]string{"yaoi": ""},
	}), http.StatusOK, &cleared)
	if cleared["googleFontsApiKey"] != "" || cleared["bookClubWebhooks"].(map[string]any)["yaoi"] != "" {
		t.Errorf("after clearing: %v", cleared)
	}

	for _, webhooks := range []map[string]string{
		{"yaoi": "https://evil.example.com/api/webhooks/1"},
		{"no-such-club": "https://discord.com/api/webhooks/1/abc"},
	} {
		resp := env.putJSON(t, "/api/v1/settings/integrations", map[string]any{"googleFontsApiKey": "", "bookClubWebhooks": webhooks})
		if _, status, _ := problem(t, resp); status != http.StatusUnprocessableEntity {
			t.Errorf("webhooks %v: %d, want 422", webhooks, status)
		}
	}
}

// The client configuration names the Turnstile site key.
func TestConfigServesTheSiteKey(t *testing.T) {
	env := newTestEnv(t)
	env.srv.SetTurnstile("sekret", "site-abc")
	var config map[string]any
	decodeInto(t, env.get(t, "/api/v1/config"), http.StatusOK, &config)
	if config["turnstileSiteKey"] != "site-abc" {
		t.Errorf("config: %v", config)
	}
}

// A theme is made, read with the stylesheet its tokens make (an unknown token and an
// injection dropped), replaced, worn, and deleted; the site's look follows.
func TestStylesLifecycle(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	var made map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/styles", map[string]any{
		"name": "Ocean", "tokens": map[string]string{"page-bg": "#08191f", "accent": "red; } body{display:none} /*", "bogus": "x"},
		"boardFlourish": "", "numberFlourish": "", "isPublic": true,
	}), http.StatusCreated, &made)
	id := int64(made["id"].(float64))
	css := made["css"].(string)
	if !strings.HasPrefix(css, ":root{--page-bg:#08191f;") || strings.Count(css, "{") != 1 || strings.Contains(css, "bogus") {
		t.Errorf("css: %q", css)
	}

	resp := env.postJSON(t, "/api/v1/styles", map[string]any{"name": "Bad", "tokens": map[string]string{}, "boardFlourish": "https://evil.example.com/x.svg", "numberFlourish": "", "isPublic": false})
	if _, status, _ := problem(t, resp); status != http.StatusUnprocessableEntity {
		t.Errorf("an outside flourish: %d, want 422", status)
	}

	var list map[string]any
	decodeInto(t, env.get(t, "/api/v1/styles"), http.StatusOK, &list)
	if list["activeStyleId"] != nil || len(list["styles"].([]any)) != 1 {
		t.Errorf("list: %v", list)
	}

	decodeInto(t, env.putJSON(t, fmt.Sprintf("/api/v1/styles/%d", id), map[string]any{
		"name": "Deep Ocean", "tokens": map[string]string{"page-bg": "#000"}, "boardFlourish": "images/flourishes/corner.svg", "numberFlourish": "", "isPublic": false,
	}), http.StatusOK, nil)

	var active map[string]any
	decodeInto(t, env.postJSON(t, fmt.Sprintf("/api/v1/styles/%d/activate", id), map[string]any{}), http.StatusOK, &active)
	decodeInto(t, env.get(t, "/api/v1/styles/active"), http.StatusOK, &active)
	if active["styleId"] != float64(id) || active["css"] != ":root{--page-bg:#000;}" || active["boardFlourish"] != "images/flourishes/corner.svg" {
		t.Errorf("the active theme: %v", active)
	}

	// It is no longer public, so players can't pick it.
	var public map[string]any
	decodeInto(t, env.get(t, "/api/v1/styles/public"), http.StatusOK, &public)
	if len(public["styles"].([]any)) != 0 {
		t.Errorf("public themes: %v", public)
	}
	if resp := env.get(t, fmt.Sprintf("/api/v1/styles/public/%d", id)); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a private theme's public read: %d, want 404", resp.StatusCode)
	}

	if resp := env.del(t, fmt.Sprintf("/api/v1/styles/%d", id)); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d, want 204", resp.StatusCode)
	}
	decodeInto(t, env.get(t, "/api/v1/styles/active"), http.StatusOK, &active)
	if active["styleId"] != nil || active["css"] != "" {
		t.Errorf("after deleting the active theme: %v", active)
	}
	for _, path := range []string{"/api/v1/styles/%d", "/api/v1/styles/%d/activate"} {
		var resp *http.Response
		if strings.HasSuffix(path, "activate") {
			resp = env.postJSON(t, fmt.Sprintf(path, id), map[string]any{})
		} else {
			resp = env.get(t, fmt.Sprintf(path, id))
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s after the delete: %d, want 404", fmt.Sprintf(path, id), resp.StatusCode)
		}
		resp.Body.Close()
	}
}

// The log reads newest first, filtered, and the level can be set; both admin-only.
func TestLogsReadAndLevel(t *testing.T) {
	env := newTestEnv(t)
	logFile := filepath.Join(t.TempDir(), "senpan.log")
	if err := os.WriteFile(logFile, []byte(
		`{"time":"2026-07-05T19:14:23Z","level":"INFO","msg":"first"}`+"\n"+
			`{"time":"2026-07-05T19:14:30Z","level":"ERROR","msg":"second"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env.srv.SetLogFile(logFile)

	if resp := env.get(t, "/api/v1/logs"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("logs signed out: %d, want 401", resp.StatusCode)
	}
	env.loginAdmin(t)

	var logs map[string]any
	decodeInto(t, env.get(t, "/api/v1/logs?level=error"), http.StatusOK, &logs)
	entries := logs["entries"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["message"] != "second" || logs["file"] != logFile {
		t.Errorf("logs at error and above: %v", logs)
	}
	if resp := env.get(t, "/api/v1/logs?limit=5000"); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("a limit over 1000: %d, want 422", resp.StatusCode)
	}

	var level map[string]any
	decodeInto(t, env.putJSON(t, "/api/v1/logs/level", map[string]string{"level": "debug"}), http.StatusOK, &level)
	t.Cleanup(func() { env.putJSON(t, "/api/v1/logs/level", map[string]string{"level": "info"}).Body.Close() })
	decodeInto(t, env.get(t, "/api/v1/logs"), http.StatusOK, &logs)
	if level["level"] != "debug" || logs["level"] != "debug" {
		t.Errorf("after setting debug: %v, %v", level, logs["level"])
	}
	if resp := env.putJSON(t, "/api/v1/logs/level", map[string]string{"level": "loud"}); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("an unknown level: %d, want 422", resp.StatusCode)
	}
}
