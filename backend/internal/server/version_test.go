package server_test

import (
	"net/http"
	"testing"

	"app-suite/internal/version"
)

// TestVersionEndpoint verifies GET /api/version is public and returns the
// backend's semantic version (the admin compatibility readout reads this).
func TestVersionEndpoint(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/version") // no login: must be public
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	body := decodeBody(t, resp)
	if got := body["backend"]; got != version.Version {
		t.Errorf("backend = %v; want %q", got, version.Version)
	}
}
