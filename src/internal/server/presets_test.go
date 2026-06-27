package server_test

import (
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
		"action": "create", "name": "Quick Game", "pattern_ids": []int64{pid}, "game_details": "GL HF",
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

	// Update.
	resp = env.postJSON(t, "/api/presets", map[string]any{
		"action": "update", "id": id, "name": "Renamed", "pattern_ids": []int64{pid},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("update status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete.
	resp = env.postJSON(t, "/api/presets", map[string]any{"action": "delete", "id": id})
	if resp.StatusCode != 200 {
		t.Fatalf("delete status = %d; want 200", resp.StatusCode)
	}
	if decodeBody(t, resp)["deleted"] != true {
		t.Error("expected deleted=true")
	}
}

func TestPresets_CreateValidation(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	pid := env.seedPattern(t, "P")

	cases := map[string]map[string]any{
		"missing name":         {"action": "create", "name": " ", "pattern_ids": []int64{pid}},
		"no patterns":          {"action": "create", "name": "Has Name", "pattern_ids": []int64{}},
		"update needs id+name": {"action": "update", "id": 0, "name": " ", "pattern_ids": []int64{pid}},
		"delete needs id":      {"action": "delete", "id": 0},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			resp := env.postJSON(t, "/api/presets", body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d; want 400", resp.StatusCode)
			}
			resp.Body.Close()
		})
	}
}

func TestPresets_InvalidAction(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	resp := env.postJSON(t, "/api/presets", map[string]any{"action": "explode"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}
