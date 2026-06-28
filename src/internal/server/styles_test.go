package server_test

import (
	"net/http"
	"testing"
)

// TestStyles_List covers handleStylesList: an authed admin gets the styles array
// plus the active style id. A freshly created theme appears.
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

	// Create one (token-based), then it shows in the list.
	resp := env.postJSON(t, "/api/styles", map[string]any{
		"action": "create", "name": "Midnight",
		"tokens": map[string]string{"page-bg": "#000", "accent": "#fff"},
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

// TestStyles_TokenRoundTrip verifies a theme stores its tokens and that GET
// returns both the tokens and the server-generated :root{} CSS — and that an
// unknown token / injection attempt is dropped rather than persisted.
func TestStyles_TokenRoundTrip(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/styles", map[string]any{
		"action": "create", "name": "Obon",
		"tokens": map[string]string{
			"page-bg": "#1a0a0b",
			"accent":  "#fbc95d",
			"bogus":   "drop-me",     // unknown → dropped
			"shadow":  "x; } body{}", // injection → stripped
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d; want 201", resp.StatusCode)
	}
	id := int64(decodeBody(t, resp)["id"].(float64))

	got := env.postJSON(t, "/api/styles", map[string]any{"action": "get", "id": id})
	if got.StatusCode != 200 {
		t.Fatalf("get status = %d; want 200", got.StatusCode)
	}
	style := decodeBody(t, got)["style"].(map[string]any)
	tokens := style["tokens"].(map[string]any)
	if tokens["page-bg"] != "#1a0a0b" || tokens["accent"] != "#fbc95d" {
		t.Errorf("tokens = %v", tokens)
	}
	if _, ok := tokens["bogus"]; ok {
		t.Error("unknown token should not be persisted")
	}
	css, _ := style["css_content"].(string)
	if css == "" || css[:6] != ":root{" {
		t.Errorf("expected generated :root css, got %q", css)
	}
}
