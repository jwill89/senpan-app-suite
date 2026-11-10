package server_test

import (
	"net/http"
	"testing"

	"app-suite/internal/version"
)

// GET /api/v1/version is public and names the version the server runs, which an open
// tab compares with its own build's.
func TestVersionEndpoint(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/v1/version") // no sign-in: it is public
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	if got := decodeBody(t, resp)["version"]; got != version.Version {
		t.Errorf("version = %v; want %q", got, version.Version)
	}
}
