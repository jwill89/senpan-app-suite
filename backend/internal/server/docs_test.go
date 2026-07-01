package server_test

import (
	"io"
	"strings"
	"testing"
)

// TestAPIDocsEndpoints verifies the public API-reference endpoints: the spec is
// served as YAML with the injected bytes, the docs page renders the Scalar
// viewer, and both are reachable without auth. When no spec is injected the spec
// endpoint reports 503 rather than serving a blank page.
func TestAPIDocsEndpoints(t *testing.T) {
	env := newTestEnv(t)
	env.srv.SetOpenAPISpec([]byte("openapi: 3.0.3\ninfo: {}\n"))

	resp := env.get(t, "/api/openapi.yaml")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET /api/openapi.yaml = %d; want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/yaml") {
		t.Errorf("spec Content-Type = %q; want application/yaml", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "openapi: 3.0.3") {
		t.Errorf("spec body did not contain the injected document")
	}

	docs := env.get(t, "/api/docs")
	defer docs.Body.Close()
	if docs.StatusCode != 200 {
		t.Fatalf("GET /api/docs = %d; want 200", docs.StatusCode)
	}
	if ct := docs.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("docs Content-Type = %q; want text/html", ct)
	}
	page, _ := io.ReadAll(docs.Body)
	if !strings.Contains(string(page), "/api/openapi.yaml") || !strings.Contains(string(page), "scalar") {
		t.Errorf("docs page did not render the Scalar viewer pointed at the spec")
	}
}

// TestAPIDocsSpecUnset confirms the spec endpoint 503s when no document is
// injected (rather than serving empty bytes).
func TestAPIDocsSpecUnset(t *testing.T) {
	env := newTestEnv(t) // no SetOpenAPISpec
	resp := env.get(t, "/api/openapi.yaml")
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatalf("GET /api/openapi.yaml with no spec = %d; want 503", resp.StatusCode)
	}
}
