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

// ── Reorder, webhook, and post-to-Discord ───────────────────────────────────

func TestAffiliates_Reorder(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Three affiliates; all share sort_order 0, so they list alphabetically first.
	idA := env.createAffiliate(t, "Alpha")
	idB := env.createAffiliate(t, "Bravo")
	idC := env.createAffiliate(t, "Charlie")

	// Drag into C, A, B order.
	resp := env.postJSON(t, "/api/affiliates/reorder", map[string]any{
		"ordered_ids": []int{idC, idA, idB},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reorder status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	list := decodeBody(t, env.get(t, "/api/affiliates"))["affiliates"].([]any)
	got := make([]int, 0, len(list))
	for _, a := range list {
		got = append(got, int(a.(map[string]any)["id"].(float64)))
	}
	want := []int{idC, idA, idB}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("order after reorder = %v; want %v", got, want)
	}
}

func TestAffiliates_Webhook(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// A valid Discord webhook saves and comes back on the list response.
	hook := "https://discord.com/api/webhooks/123/abc"
	resp := env.putJSON(t, "/api/affiliates/webhook", map[string]any{"webhook_url": hook})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set webhook status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
	if got := decodeBody(t, env.get(t, "/api/affiliates"))["webhook_url"]; got != hook {
		t.Errorf("webhook_url = %v; want %q", got, hook)
	}

	// A non-Discord URL is rejected.
	resp = env.putJSON(t, "/api/affiliates/webhook", map[string]any{"webhook_url": "https://evil.example.com/x"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad webhook status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAffiliates_Post(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)
	id := env.createAffiliate(t, "Poster")

	// Posting with no webhook configured is a 400.
	resp := env.postJSON(t, fmt.Sprintf("/api/affiliates/%d/post", id), map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("post without webhook status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Posting an unknown affiliate is a 404 (even with a webhook set).
	env.putJSON(t, "/api/affiliates/webhook",
		map[string]any{"webhook_url": "https://discord.com/api/webhooks/1/x"}).Body.Close()
	resp = env.postJSON(t, "/api/affiliates/999999/post", map[string]any{})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("post unknown affiliate status = %d; want 404", resp.StatusCode)
	}
	resp.Body.Close()
}
