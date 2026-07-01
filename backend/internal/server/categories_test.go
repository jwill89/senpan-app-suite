package server_test

import (
	"fmt"
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
	resp := env.postJSON(t, "/api/pattern-categories", map[string]any{"name": "X"})
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

	// Create (POST /api/pattern-categories → 201).
	resp := env.postJSON(t, "/api/pattern-categories", map[string]any{"name": "Bonus"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d; want 201", resp.StatusCode)
	}
	id := int64(decodeBody(t, resp)["id"].(float64))

	after := decodeBody(t, env.get(t, "/api/pattern-categories"))["categories"].([]any)
	if len(after) != len(before)+1 {
		t.Fatalf("expected %d categories, got %d", len(before)+1, len(after))
	}

	// Rename (PATCH /api/pattern-categories/{id} → 200).
	resp = env.patchJSON(t, fmt.Sprintf("/api/pattern-categories/%d", id), map[string]any{"name": "Bonus Patterns"})
	if resp.StatusCode != 200 {
		t.Fatalf("rename status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete (not the last → 204).
	resp = env.del(t, fmt.Sprintf("/api/pattern-categories/%d", id))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d; want 204", resp.StatusCode)
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

	// Deleting the last remaining category is refused with 409.
	resp := env.del(t, fmt.Sprintf("/api/pattern-categories/%d", lastID))
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("delete-last status = %d; want 409", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCategories_ReorderBulk(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Seed a couple more categories so there's an order to permute.
	first := decodeBody(t, env.get(t, "/api/pattern-categories"))["categories"].([]any)
	firstID := int64(first[0].(map[string]any)["id"].(float64))
	bID := int64(decodeBody(t, env.postJSON(t, "/api/pattern-categories", map[string]any{"name": "B"}))["id"].(float64))

	// Bulk reorder: B before the seed. Returns the fresh, reordered list.
	resp := env.postJSON(t, "/api/pattern-categories/reorder", map[string]any{
		"ordered_ids": []int64{bID, firstID},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reorder status = %d; want 200", resp.StatusCode)
	}
	cats := decodeBody(t, resp)["categories"].([]any)
	if int64(cats[0].(map[string]any)["id"].(float64)) != bID {
		t.Errorf("first category id = %v; want B (%d)", cats[0].(map[string]any)["id"], bID)
	}
}

func TestCategories_Validation(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	t.Run("empty create name", func(t *testing.T) {
		resp := env.postJSON(t, "/api/pattern-categories", map[string]any{"name": "  "})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d; want 400", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("empty rename name", func(t *testing.T) {
		id := int64(decodeBody(t, env.get(t, "/api/pattern-categories"))["categories"].([]any)[0].(map[string]any)["id"].(float64))
		resp := env.patchJSON(t, fmt.Sprintf("/api/pattern-categories/%d", id), map[string]any{"name": " "})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d; want 400", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("empty patch body", func(t *testing.T) {
		id := int64(decodeBody(t, env.get(t, "/api/pattern-categories"))["categories"].([]any)[0].(map[string]any)["id"].(float64))
		resp := env.patchJSON(t, fmt.Sprintf("/api/pattern-categories/%d", id), map[string]any{})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d; want 400", resp.StatusCode)
		}
		resp.Body.Close()
	})

	t.Run("bad reorder direction", func(t *testing.T) {
		id := int64(decodeBody(t, env.get(t, "/api/pattern-categories"))["categories"].([]any)[0].(map[string]any)["id"].(float64))
		resp := env.patchJSON(t, fmt.Sprintf("/api/pattern-categories/%d", id), map[string]any{"direction": "sideways"})
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d; want 400", resp.StatusCode)
		}
		resp.Body.Close()
	})
}
