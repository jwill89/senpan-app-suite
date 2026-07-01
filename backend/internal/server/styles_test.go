package server_test

import (
	"fmt"
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

	// Create one (token-based, POST /api/styles), then it shows in the list.
	resp := env.postJSON(t, "/api/styles", map[string]any{
		"name":   "Midnight",
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
// /api/styles/{id} returns both the tokens and the server-generated :root{} CSS —
// and that an unknown token / injection attempt is dropped rather than persisted.
func TestStyles_TokenRoundTrip(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/styles", map[string]any{
		"name": "Obon",
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

	got := env.get(t, fmt.Sprintf("/api/styles/%d", id))
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

// TestStyles_GetNotFound verifies GET /api/styles/{id} 404s for a missing style.
func TestStyles_GetNotFound(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.get(t, "/api/styles/99999")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestStyles_UpdateReplaceAndDelete covers PUT /api/styles/{id} (full replace,
// name required) and DELETE /api/styles/{id} (204).
func TestStyles_UpdateReplaceAndDelete(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/styles", map[string]any{
		"name": "Draft", "tokens": map[string]string{"accent": "#111"},
	})
	id := int64(decodeBody(t, resp)["id"].(float64))

	// Replace (PUT).
	resp = env.putJSON(t, fmt.Sprintf("/api/styles/%d", id), map[string]any{
		"name": "Final", "tokens": map[string]string{"accent": "#222"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Name is required on replace.
	resp = env.putJSON(t, fmt.Sprintf("/api/styles/%d", id), map[string]any{"name": "  "})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty-name update status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete (204).
	resp = env.del(t, fmt.Sprintf("/api/styles/%d", id))
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()

	// Gone.
	resp = env.get(t, fmt.Sprintf("/api/styles/%d", id))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("deleted style should 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestStyles_ActivateDeactivate covers POST /api/styles/{id}/activate and
// POST /api/styles/deactivate, checking the active_style_id setting flips and the
// public /api/styles/active CSS follows.
func TestStyles_ActivateDeactivate(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/styles", map[string]any{
		"name": "Live", "tokens": map[string]string{"accent": "#abcdef"},
	})
	id := int64(decodeBody(t, resp)["id"].(float64))

	// Activate.
	resp = env.postJSON(t, fmt.Sprintf("/api/styles/%d/activate", id), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("activate status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	list := decodeBody(t, env.get(t, "/api/styles"))
	if list["active_style_id"] != fmt.Sprintf("%d", id) {
		t.Errorf("active_style_id = %v; want %d", list["active_style_id"], id)
	}
	active := decodeBody(t, env.get(t, "/api/styles/active"))
	if css, _ := active["css"].(string); css == "" {
		t.Error("expected non-empty active CSS after activation")
	}

	// Deactivate.
	resp = env.postJSON(t, "/api/styles/deactivate", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("deactivate status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	list = decodeBody(t, env.get(t, "/api/styles"))
	if list["active_style_id"] != "" {
		t.Errorf("active_style_id = %v; want empty after deactivate", list["active_style_id"])
	}
	active = decodeBody(t, env.get(t, "/api/styles/active"))
	if active["css"] != "" {
		t.Errorf("expected empty active CSS after deactivate, got %v", active["css"])
	}
}
