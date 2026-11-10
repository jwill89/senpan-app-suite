package server_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
)

// preset is a full preset body with the given name and patterns.
func preset(name string, ids ...int64) map[string]any {
	return map[string]any{"name": name, "patternIds": ids, "gameDetails": "GL HF", "auto": true, "autoInterval": 45}
}

// The Presets page's permission guards every operation, and its grantee gets
// through.
func TestPresetsGuard(t *testing.T) {
	env := newTestEnv(t)
	if got := status(env.get(t, "/api/v1/presets")); got != http.StatusUnauthorized {
		t.Errorf("signed out: %d, want 401", got)
	}
	addAccount(t, env, "patternsonly", "correct horse battery", []string{"bingo-patterns"})
	env.signIn(t, "patternsonly", "correct horse battery")
	if got := status(env.get(t, "/api/v1/presets")); got != http.StatusForbidden {
		t.Errorf("without the page: %d, want 403", got)
	}
	if got := status(env.postJSON(t, "/api/v1/presets", preset("X", 1))); got != http.StatusForbidden {
		t.Errorf("a save without the page: %d, want 403", got)
	}

	addAccount(t, env, "presetter", "correct horse battery", []string{"bingo-presets"})
	env.signIn(t, "presetter", "correct horse battery")
	decodeInto(t, env.postJSON(t, "/api/v1/presets", preset("Mine", 1)), http.StatusCreated, nil)
}

// A preset is made (201, in full), listed, replaced, and deleted (204, then 404);
// the automatic-draw interval is held within its bounds.
func TestPresetsLifecycle(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	pid := env.seedPattern(t, "Diagonal")

	var made map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/presets", preset(" Quick Game ", pid)), http.StatusCreated, &made)
	if made["name"] != "Quick Game" || fmt.Sprint(made["patternIds"]) != fmt.Sprint([]any{float64(pid)}) ||
		made["gameDetails"] != "GL HF" || made["auto"] != true || made["autoInterval"] != float64(45) || made["createdAt"] == nil {
		t.Errorf("made: %v", made)
	}
	id := int64(made["id"].(float64))
	path := fmt.Sprintf("/api/v1/presets/%d", id)

	var list struct {
		Presets []map[string]any `json:"presets"`
	}
	decodeInto(t, env.get(t, "/api/v1/presets"), http.StatusOK, &list)
	if len(list.Presets) != 1 || list.Presets[0]["id"] != made["id"] {
		t.Fatalf("list: %v", list.Presets)
	}

	var saved map[string]any
	decodeInto(t, env.putJSON(t, path, map[string]any{
		"name": "Renamed", "patternIds": []int64{pid, 99}, "gameDetails": "", "auto": false, "autoInterval": 2,
	}), http.StatusOK, &saved)
	if saved["name"] != "Renamed" || saved["auto"] != false || saved["gameDetails"] != "" ||
		saved["autoInterval"] != float64(5) || len(saved["patternIds"].([]any)) != 2 {
		t.Errorf("replaced: %v", saved)
	}
	decodeInto(t, env.putJSON(t, path, preset("Default interval", pid)), http.StatusOK, nil)
	body := preset("Default interval", pid)
	body["autoInterval"] = 0
	decodeInto(t, env.putJSON(t, path, body), http.StatusOK, &saved)
	if saved["autoInterval"] != float64(30) {
		t.Errorf("an interval of 0: %v, want the default 30", saved["autoInterval"])
	}

	if got := status(env.putJSON(t, "/api/v1/presets/999", preset("Lost", pid))); got != http.StatusNotFound {
		t.Errorf("replacing an unknown preset: %d, want 404", got)
	}
	if got := status(env.del(t, path)); got != http.StatusNoContent {
		t.Fatalf("delete: %d, want 204", got)
	}
	if got := status(env.del(t, path)); got != http.StatusNotFound {
		t.Errorf("deleting it again: %d, want 404", got)
	}
}

// A blank name and no patterns are each a 422 naming the field, on a create and a
// replace alike.
func TestPresetsValidation(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	pid := env.seedPattern(t, "P")
	var made map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/presets", preset("Fine", pid)), http.StatusCreated, &made)
	path := fmt.Sprintf("/api/v1/presets/%d", int64(made["id"].(float64)))

	for _, c := range []struct {
		body map[string]any
		want string
	}{
		{preset(" ", pid), "body.name"},
		{preset("No Patterns"), "body.patternIds"},
	} {
		if at := invalid(t, env.postJSON(t, "/api/v1/presets", c.body)); !slices.Contains(at, c.want) {
			t.Errorf("create %v: errors at %v, want %s", c.body, at, c.want)
		}
		if at := invalid(t, env.putJSON(t, path, c.body)); !slices.Contains(at, c.want) {
			t.Errorf("replace %v: errors at %v, want %s", c.body, at, c.want)
		}
	}
}
