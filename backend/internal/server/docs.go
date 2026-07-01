package server

import "net/http"

// This file serves the API reference: the OpenAPI document plus a Scalar viewer.
// These are documentation infrastructure, not part of the documented API surface,
// so they are registered separately (registerDocs, called from New) rather than
// in routes() — which keeps routes() the single, authoritative list of API
// endpoints that the OpenAPI route-coverage test checks against.

// openAPISpec holds the committed openapi.yaml bytes, injected from main via
// SetOpenAPISpec (embedded there so the server binary is self-contained). Empty
// until set — the docs endpoints then 503 rather than serving a blank page.
// (Field lives on Server; see server.go.)

// SetOpenAPISpec provides the embedded OpenAPI document served at
// GET /api/openapi.yaml (and rendered by GET /api/docs).
func (s *Server) SetOpenAPISpec(spec []byte) { s.openAPISpec = spec }

// registerDocs wires the public API-reference endpoints onto the mux.
func (s *Server) registerDocs() {
	s.mux.HandleFunc("GET /api/openapi.yaml", s.handleOpenAPISpec)
	s.mux.HandleFunc("GET /api/docs", s.handleAPIDocs)
}

// handleOpenAPISpec serves the raw OpenAPI 3 document (public — the API contract
// is already public in the repo; the spec carries no secrets).
func (s *Server) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	if len(s.openAPISpec) == 0 {
		http.Error(w, "API spec not available", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(s.openAPISpec)
}

// handleAPIDocs serves a minimal HTML page that renders the spec with Scalar's
// standalone API-reference viewer (loaded from a CDN). Public.
func (s *Server) handleAPIDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(scalarHTML))
}

// scalarHTML embeds Scalar's standalone viewer pointed at the spec endpoint. The
// same shell is reused across projects — only data-url changes.
const scalarHTML = `<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Senpan App Suite — API Reference</title>
  </head>
  <body>
    <script id="api-reference" data-url="/api/openapi.yaml"></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
  </body>
</html>
`
