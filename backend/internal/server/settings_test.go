package server_test

import (
	"net/http"
	"testing"
)

func settingsMap(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	data := decodeBody(t, resp)
	m, _ := data["settings"].(map[string]any)
	if m == nil {
		t.Fatal("settings response carried no settings map")
	}
	return m
}

// TestSettingsSaveKeepsSecretsFromNonAdmin pins the read and write paths to the
// same rule. GET /api/settings blanks secretSettings for anyone who is not a full
// admin, but POST admits any holder of the grantable system-settings page - so a
// non-admin loaded the form with the Google Fonts key and every club webhook
// blank and, on save, wrote those blanks straight over the stored secrets. An
// unrelated edit to the app title silently destroyed them.
func TestSettingsSaveKeepsSecretsFromNonAdmin(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	const secret = "fonts-api-key-do-not-clobber"
	resp := env.postJSON(t, "/api/settings", map[string]any{
		"settings": map[string]string{"google_fonts_api_key": secret},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin seed save status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	staff := makeActiveUser(t, env, "settingsstaff", "correct horse battery", []string{"system-settings"})

	// The premise: the grantee genuinely cannot see the secret.
	got := settingsMap(t, getAs(t, staff, env, "/api/settings"))
	if got["google_fonts_api_key"] != "" {
		t.Fatalf("non-admin GET leaked the secret: %v", got["google_fonts_api_key"])
	}

	// They save the page, echoing back the blank they were shown alongside a real
	// edit of their own - exactly what the settings form does.
	resp = postAs(t, staff, env, "/api/settings", map[string]any{
		"settings": map[string]string{
			"app_title":            "Renamed By Staff",
			"google_fonts_api_key": "",
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("non-admin save status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	env.loginAdmin(t)
	after := settingsMap(t, env.get(t, "/api/settings"))
	if after["google_fonts_api_key"] != secret {
		t.Errorf("secret after non-admin save = %q; want %q (a blanked value was saved over it)",
			after["google_fonts_api_key"], secret)
	}
	// The edit they were actually allowed to make must still land, or the guard
	// has broken the page for the very users the permission exists to serve.
	if after["app_title"] != "Renamed By Staff" {
		t.Errorf("app_title = %v; want the non-admin's edit to have been saved", after["app_title"])
	}
}

// TestSettingsSaveAdminCanStillClearSecret guards the other direction: the skip is
// scoped to non-admins, so an admin clearing a secret on purpose still works.
func TestSettingsSaveAdminCanStillClearSecret(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/settings", map[string]any{
		"settings": map[string]string{"google_fonts_api_key": "temporary"},
	})
	resp.Body.Close()

	resp = env.postJSON(t, "/api/settings", map[string]any{
		"settings": map[string]string{"google_fonts_api_key": ""},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin clear status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	after := settingsMap(t, env.get(t, "/api/settings"))
	if after["google_fonts_api_key"] != "" {
		t.Errorf("admin clear did not take: %v", after["google_fonts_api_key"])
	}
}
