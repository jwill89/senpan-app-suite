package server_test

import (
	"net/http"
	"testing"
)

// TestStyles_List covers handleStylesList: an authed admin gets the styles array
// (without CSS bodies) plus the active style id. A freshly created style appears.
func TestStyles_List(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Empty to start.
	data := decodeBody(t, env.get(t, "/api/styles"))
	if _, ok := data["styles"]; !ok {
		t.Fatal("response missing styles key")
	}
	if _, ok := data["active_style_id"]; !ok {
		t.Fatal("response missing active_style_id key")
	}

	// Create one, then it shows in the list.
	resp := env.postJSON(t, "/api/styles", map[string]any{
		"action": "create", "name": "Midnight", "css_content": ":root{}",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create style status = %d; want 201", resp.StatusCode)
	}
	resp.Body.Close()

	styles := decodeBody(t, env.get(t, "/api/styles"))["styles"].([]any)
	if len(styles) != 1 {
		t.Fatalf("expected 1 style, got %d", len(styles))
	}
}
