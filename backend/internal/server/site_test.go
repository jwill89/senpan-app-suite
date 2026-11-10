package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"app-suite/internal/version"
)

// GET /healthz answers with the version running and the schema's, for the deploy's
// checks and an uptime monitor.
func TestHealthzReportsTheVersionAndSchema(t *testing.T) {
	env := newTestEnv(t)
	resp := env.get(t, "/healthz")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz: %d", resp.StatusCode)
	}
	var body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
		Schema  int    `json:"schema"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Version != version.Version || body.Schema < 1 {
		t.Fatalf("GET /healthz: %+v", body)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("GET /healthz: Cache-Control %q", got)
	}
}

// Outside /api/ the server hands every path to the SPA; the API stays the API's.
func TestPathsOutsideTheAPIGoToTheSite(t *testing.T) {
	env := newTestEnv(t)
	if resp := env.get(t, "/"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET / with no site: %d, want 404", resp.StatusCode)
	}

	env.srv.SetSite(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "site:"+r.URL.Path)
	}))
	for _, path := range []string{"/", "/admin/bingo", "/assets/index-abc.js", "/apiary"} {
		resp := env.get(t, path)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || string(body) != "site:"+path {
			t.Fatalf("GET %s: %d %q, want the site", path, resp.StatusCode, body)
		}
	}
	resp := env.get(t, "/api/v1/version")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), version.Version) {
		t.Fatalf("GET /api/v1/version: %q, want the API's answer", body)
	}
	if resp := env.get(t, "/api/no-such-route"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /api/no-such-route: %d, want the API's 404", resp.StatusCode)
	}
}
