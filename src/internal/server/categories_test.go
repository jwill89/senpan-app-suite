package server_test

import (
	"net/http"
	"testing"
)

// Pattern categories live at /api/pattern-categories (distinct from the pattern
// CRUD at /api/patterns). A fresh store is seeded with a default "Standard"
// category, so the last-category delete guard is exercisable.

func TestCategories_RequiresAuth(t *testing.T) {
	env := newTestEnv(t)
	if resp := env.get(t, "/api/pattern-categories"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("list status = %d; want 401", resp.StatusCode)
		resp.Body.Close()
	}
	resp := env.postJSON(t, "/api/pattern-categories", map[string]any{"action": "create", "name": "X"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("create status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCategories_CreateListRenameDelete(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// The default "Standard" category is present.
	before := decodeBody(t, env.get(t, "/api/pattern-categories"))["categories"].([]any)
	if len(before) == 0 {
		t.Fatal("expected a seeded default category")
	}

	// Create.
	resp := env.postJSON(t, "/api/pattern-categories", map[string]any{"action": "create", "name": "Bonus"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d; want 201", resp.StatusCode)
	}
	id := int64(decodeBody(t, resp)["id"].(float64))

	after := decodeBody(t, env.get(t, "/api/pattern-categories"))["categories"].([]any)
	if len(after) != len(before)+1 {
		t.Fatalf("expected %d categories, got %d", len(before)+1, len(after))
	}

	// Rename.
	resp = env.postJSON(t, "/api/pattern-categories", map[string]any{"action": "rename", "id": id, "name": "Bonus Patterns"})
	if resp.StatusCode != 200 {
		t.Fatalf("rename status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete (not the last → ok).
	resp = env.postJSON(t, "/api/pattern-categories", map[string]any{"action": "delete", "id": id})
	if resp.StatusCode != 200 {
		t.Fatalf("delete status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCategories_CannotDeleteLast(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	cats := decodeBody(t, env.get(t, "/api/pattern-categories"))["categories"].([]any)
	if len(cats) != 1 {
		t.Skipf("expected exactly 1 seeded category, got %d", len(cats))
	}
	lastID := int64(cats[0].(map[string]any)["id"].(float64))

	resp := env.postJSON(t, "/api/pattern-categories", map[string]any{"action": "delete", "id": lastID})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("delete-last status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCategories_Validation(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	cases := map[string]map[string]any{
		"empty name":        {"action": "create", "name": "  "},
		"rename needs both": {"action": "rename", "id": 0, "name": " "},
		"delete needs id":   {"action": "delete", "id": 0},
		"invalid action":    {"action": "explode"},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			resp := env.postJSON(t, "/api/pattern-categories", body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d; want 400", resp.StatusCode)
			}
			resp.Body.Close()
		})
	}
}
