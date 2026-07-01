package server_test

import (
	"fmt"
	"net/http"
	"testing"
)

// seedPattern creates one win pattern (in the default "Standard" category) and
// returns its id, so preset tests have a valid pattern to reference.
func (e *testEnv) seedPattern(t *testing.T, name string) int64 {
	t.Helper()
	id, err := e.store.SavePattern(name, [][]bool{
		{true, false, false, false, false},
		{false, true, false, false, false},
		{false, false, true, false, false},
		{false, false, false, true, false},
		{false, false, false, false, true},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPresets_RequiresAuth(t *testing.T) {
	env := newTestEnv(t)
	if resp := env.get(t, "/api/presets"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("list status = %d; want 401", resp.StatusCode)
		resp.Body.Close()
	}
	resp := env.postJSON(t, "/api/presets", map[string]any{"action": "create", "name": "X"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("create status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPresets_CreateListUpdateDelete(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	pid := env.seedPattern(t, "Diagonal")

	// Create.
	resp := env.postJSON(t, "/api/presets", map[string]any{
		"name": "Quick Game", "pattern_ids": []int64{pid}, "game_details": "GL HF",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d; want 201", resp.StatusCode)
	}
	id := int64(decodeBody(t, resp)["id"].(float64))

	// List shows it.
	presets := decodeBody(t, env.get(t, "/api/presets"))["presets"].([]any)
	if len(presets) != 1 {
		t.Fatalf("expected 1 preset, got %d", len(presets))
	}

	// Update (PUT).
	resp = env.putJSON(t, fmt.Sprintf("/api/presets/%d", id), map[string]any{
		"name": "Renamed", "pattern_ids": []int64{pid},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("update status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete (DELETE → 204).
	resp = env.del(t, fmt.Sprintf("/api/presets/%d", id))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestPresets_CreateValidation(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	pid := env.seedPattern(t, "P")

	// Create validation (POST /api/presets).
	create := map[string]map[string]any{
		"missing name": {"name": " ", "pattern_ids": []int64{pid}},
		"no patterns":  {"name": "Has Name", "pattern_ids": []int64{}},
	}
	for name, body := range create {
		t.Run(name, func(t *testing.T) {
			resp := env.postJSON(t, "/api/presets", body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d; want 400", resp.StatusCode)
			}
			resp.Body.Close()
		})
	}

	// Update validation (PUT /api/presets/{id}): a blank name is rejected.
	t.Run("update blank name", func(t *testing.T) {
		resp := env.putJSON(t, fmt.Sprintf("/api/presets/%d", pid),
			map[string]any{"name": " ", "pattern_ids": []int64{pid}})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d; want 400", resp.StatusCode)
		}
		resp.Body.Close()
	})
}
