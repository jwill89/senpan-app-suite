package server_test

import (
	"fmt"
	"net/http"
	"testing"

	"app-suite/internal/auth"
)

// ── Affiliates admin CRUD ───────────────────────────────────────────────────

func TestAffiliates_RequiresAuth(t *testing.T) {
	env := newTestEnv(t)

	if resp := env.get(t, "/api/affiliates"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("list status = %d; want 401", resp.StatusCode)
		resp.Body.Close()
	}
	resp := env.postJSON(t, "/api/affiliates", map[string]any{"action": "create", "name": "X"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("create status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

// createAffiliate posts a valid create and returns the new affiliate's id.
func (e *testEnv) createAffiliate(t *testing.T, name string) int {
	t.Helper()
	resp := e.postJSON(t, "/api/affiliates", map[string]any{
		"action":   "create",
		"name":     name,
		"owners":   []string{"Tataru", "  ", "Hildibrand"}, // blank dropped
		"location": "Ul'dah",
		"timezone": "America/New_York",
		"hours": []map[string]any{
			{"label": "Mon–Fri", "start": "18:00", "end": "23:00"},
			{"label": "Closed row", "start": "  "}, // dropped (no start)
		},
		"details":    "Cozy.",
		"logo":       "images/affiliate_logos/x.png",
		"screenshot": "images/affiliate_images/x.png",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create affiliate status = %d; want 201", resp.StatusCode)
	}
	a := decodeBody(t, resp)["affiliate"].(map[string]any)
	return int(a["id"].(float64))
}

func TestAffiliates_CreateListAndSanitize(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	env.createAffiliate(t, "The Tipsy Moogle")

	list := decodeBody(t, env.get(t, "/api/affiliates"))["affiliates"].([]any)
	if len(list) != 1 {
		t.Fatalf("expected 1 affiliate, got %d", len(list))
	}
	a := list[0].(map[string]any)
	// Blank owner + the start-less hours row are sanitized away.
	owners := a["owners"].([]any)
	if len(owners) != 2 {
		t.Errorf("owners = %v; want 2 (blank dropped)", owners)
	}
	hours := a["hours"].([]any)
	if len(hours) != 1 {
		t.Errorf("hours = %v; want 1 (start-less row dropped)", hours)
	}
}

func TestAffiliates_CreateValidation(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/affiliates", map[string]any{"action": "create", "name": "  "})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing name status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAffiliates_UpdateAndDelete(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	id := env.createAffiliate(t, "Original")

	// Update (PUT /api/affiliates/{id}).
	resp := env.putJSON(t, fmt.Sprintf("/api/affiliates/%d", id), map[string]any{
		"name": "Renamed", "owners": []string{"Solo"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	list := decodeBody(t, env.get(t, "/api/affiliates"))["affiliates"].([]any)
	if list[0].(map[string]any)["name"] != "Renamed" {
		t.Errorf("name = %v; want Renamed", list[0].(map[string]any)["name"])
	}

	// Delete (DELETE /api/affiliates/{id} → 204).
	resp = env.del(t, fmt.Sprintf("/api/affiliates/%d", id))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()

	if list := decodeBody(t, env.get(t, "/api/affiliates"))["affiliates"].([]any); len(list) != 0 {
		t.Errorf("expected affiliate deleted, got %d remaining", len(list))
	}
}

func TestAffiliates_UpdateValidation(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	id := env.createAffiliate(t, "Original")

	// A blank name is rejected on update (PUT /api/affiliates/{id}).
	resp := env.putJSON(t, fmt.Sprintf("/api/affiliates/%d", id),
		map[string]any{"name": "  ", "owners": []string{"Solo"}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("update blank name status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestAffiliates_PermissionGating verifies a non-admin account needs the
// teahouse-affiliates page permission specifically: a different grant gets 403,
// and granting teahouse-affiliates opens access. Permissions are toggled directly
// via the store and read fresh on each request (currentUser reloads per request),
// so a single logged-in staff session exercises both states.
func TestAffiliates_PermissionGating(t *testing.T) {
	env := newTestEnv(t)

	hash, err := auth.Hash("password123")
	if err != nil {
		t.Fatal(err)
	}
	staff, err := env.store.CreateUser("staff", hash)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.store.SetUserActive(staff.ID, true); err != nil {
		t.Fatal(err)
	}
	// Grant an unrelated permission first → still forbidden on affiliates.
	if err := env.store.SetUserPermissions(staff.ID, []string{"bingo-cards"}); err != nil {
		t.Fatal(err)
	}

	env.postJSON(t, "/api/auth", map[string]string{
		"action": "login", "username": "staff", "password": "password123",
	}).Body.Close()

	if resp := env.get(t, "/api/affiliates"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("wrong-permission status = %d; want 403", resp.StatusCode)
		resp.Body.Close()
	}

	// Grant teahouse-affiliates → the next request reloads perms and is allowed.
	if err := env.store.SetUserPermissions(staff.ID, []string{"teahouse-affiliates"}); err != nil {
		t.Fatal(err)
	}
	if resp := env.get(t, "/api/affiliates"); resp.StatusCode != http.StatusOK {
		t.Errorf("granted status = %d; want 200", resp.StatusCode)
		resp.Body.Close()
	}
}
