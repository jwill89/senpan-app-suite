package server_test

import (
	"net/http"
	"testing"
)

// A raffle's window is stored in the one timestamp form: the browser's
// milliseconds come back in UTC to the second, and a value that isn't a time is
// a 400 rather than a write the column's CHECK refuses.
func TestRaffleWindowIsStoredInTheOneForm(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	resp := env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Window", "max_entries": 1,
		"available_from": "next tuesday",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a raffle open from 'next tuesday': status %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	resp = env.postJSON(t, "/api/raffles", map[string]any{
		"action": "create", "title": "Window", "max_entries": 1,
		"available_from": "2026-06-13T22:00:00.000+02:00", "available_to": "",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: status %d, want 201", resp.StatusCode)
	}
	raffle := decodeBody(t, resp)["raffle"].(map[string]any)
	if raffle["available_from"] != "2026-06-13T20:00:00Z" || raffle["available_to"] != "" {
		t.Errorf("window = %v to %v; want 2026-06-13T20:00:00Z to none (\"\")", raffle["available_from"], raffle["available_to"])
	}
}
