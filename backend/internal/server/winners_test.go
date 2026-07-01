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

func TestWinnersLog_DeleteAndDeleteAll(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	// DELETE /api/winners-log/{id}: deleting a non-existent entry is a no-op
	// success (idempotent) → 204.
	resp := env.del(t, "/api/winners-log/12345")
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("delete status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()

	// DELETE /api/winners-log/all clears the log and returns the deleted count
	// (0 on an empty log) → 200.
	body := decodeBody(t, env.del(t, "/api/winners-log/all"))
	if _, ok := body["deleted"]; !ok {
		t.Errorf("delete_all response missing 'deleted' count: %v", body)
	}
}
