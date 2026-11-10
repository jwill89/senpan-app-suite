package server_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestPublicStylesEndpoints(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	theme := func(name string, public bool) int64 {
		var made map[string]any
		decodeInto(t, env.postJSON(t, "/api/v1/styles", map[string]any{
			"name": name, "tokens": map[string]string{"page-bg": "#08191f"}, "boardFlourish": "", "numberFlourish": "", "isPublic": public,
		}), http.StatusCreated, &made)
		return int64(made["id"].(float64))
	}
	pubID, privID := theme("Ocean Public", true), theme("Secret", false)

	var list map[string]any
	decodeInto(t, env.get(t, "/api/v1/styles/public"), http.StatusOK, &list)
	styles, _ := list["styles"].([]any)
	if len(styles) != 1 || styles[0].(map[string]any)["name"] != "Ocean Public" {
		t.Fatalf("public styles = %v; want only Ocean Public", styles)
	}
	var css map[string]any
	decodeInto(t, env.get(t, fmt.Sprintf("/api/v1/styles/public/%d", pubID)), http.StatusOK, &css)
	if css["css"] != ":root{--page-bg:#08191f;}" {
		t.Errorf("public theme css = %v", css["css"])
	}
	if resp := env.get(t, fmt.Sprintf("/api/v1/styles/public/%d", privID)); resp.StatusCode != http.StatusNotFound {
		t.Errorf("private theme: %d; want 404", resp.StatusCode)
	}
}

// The custom card cost reaches the public settings as a number of gil, and a cost
// out of range is refused.
func TestCustomCardCostSetting(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	if s := siteSettings(t, env); s["customCardCost"] != float64(0) {
		t.Fatalf("default cost = %v; want 0", s["customCardCost"])
	}
	decodeInto(t, saveSettings(t, env, map[string]any{"customCardCost": 5000000}), http.StatusOK, nil)
	if s := siteSettings(t, env); s["customCardCost"] != float64(5000000) {
		t.Fatalf("cost = %v; want 5000000", s["customCardCost"])
	}
	for _, bad := range []any{-1, 1000000001, "lots"} {
		resp := saveSettings(t, env, map[string]any{"customCardCost": bad})
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("cost %v: %d; want 422", bad, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

// The two hide flags are true or false, reach the public settings, and refuse
// anything else.
func TestHideSettings(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	for _, field := range []string{"hideBingo", "hideCustomCards"} {
		if s := siteSettings(t, env); s[field] != false {
			t.Fatalf("%s default = %v; want false", field, s[field])
		}
		decodeInto(t, saveSettings(t, env, map[string]any{field: true}), http.StatusOK, nil)
		if s := siteSettings(t, env); s[field] != true {
			t.Fatalf("%s after a save = %v; want true", field, s[field])
		}
		resp := saveSettings(t, env, map[string]any{field: "1"})
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s = \"1\": %d; want 422", field, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
