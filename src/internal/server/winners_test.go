package server_test

import (
	"net/http"
	"testing"
)

// TestWinnersLog_Frequent exercises the frequent-winners endpoint with the
// default thresholds (no settings rows). With no winners it returns an empty list.
func TestWinnersLog_Frequent(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	data := decodeBody(t, env.get(t, "/api/winners-log/frequent"))
	winners, ok := data["winners"]
	if !ok {
		t.Fatal("response missing winners key")
	}
	if list, _ := winners.([]any); len(list) != 0 {
		t.Errorf("expected no frequent winners, got %d", len(list))
	}
}

func TestWinnersLog_Frequent_RequiresAuth(t *testing.T) {
	env := newTestEnv(t)
	if resp := env.get(t, "/api/winners-log/frequent"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", resp.StatusCode)
		resp.Body.Close()
	}
}

func TestWinnersLog_ActionValidation(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// delete without an id → 400.
	resp := env.postJSON(t, "/api/winners-log", map[string]any{"action": "delete", "id": 0})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("delete (no id) status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// unknown action → 400.
	resp = env.postJSON(t, "/api/winners-log", map[string]any{"action": "explode"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid action status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestWinnersLog_DeleteAndDeleteAll(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// Deleting a non-existent entry is a no-op success (idempotent).
	resp := env.postJSON(t, "/api/winners-log", map[string]any{"action": "delete", "id": 12345})
	if resp.StatusCode != 200 {
		t.Errorf("delete status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// delete_all on an empty log also succeeds.
	resp = env.postJSON(t, "/api/winners-log", map[string]any{"action": "delete_all"})
	if resp.StatusCode != 200 {
		t.Errorf("delete_all status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
}
