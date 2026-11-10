package server_test

import (
	"net/http"
	"testing"
)

// A raffle's window is stored in the one timestamp form: the browser's
// milliseconds come back in UTC to the second, and a value that isn't a time is
// a 422 rather than a write the column's CHECK refuses.
func TestRaffleWindowIsStoredInTheOneForm(t *testing.T) {
	env := newTestEnv(t)
	env.loginAdmin(t)

	if at := invalid(t, env.postJSON(t, "/api/v1/raffles", raffleBody(map[string]any{
		"maxEntries": 1, "availableFrom": "next tuesday",
	}))); len(at) == 0 {
		t.Error("a raffle open from 'next tuesday' named no field")
	}

	var raffle map[string]any
	decodeInto(t, env.postJSON(t, "/api/v1/raffles", raffleBody(map[string]any{
		"maxEntries": 1, "availableFrom": "2026-06-13T22:00:00.000+02:00", "availableTo": nil,
	})), http.StatusCreated, &raffle)
	if raffle["availableFrom"] != "2026-06-13T20:00:00Z" || raffle["availableTo"] != nil {
		t.Errorf("window = %v to %v; want 2026-06-13T20:00:00Z to none (null)", raffle["availableFrom"], raffle["availableTo"])
	}
}
