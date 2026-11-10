package server

import (
	"encoding/json"
	"net/http"

	"app-suite/internal/version"
)

// handleHealth is GET /healthz: whether the app and its database answer, with the
// version running and the schema's. Public, and outside the session, CSRF and
// access-log layers, so the deploy's checks and an uptime monitor can poll it
// freely. A database that doesn't answer is a 503.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	status, state := http.StatusOK, "ok"
	schema, err := s.store.SchemaVersion(r.Context())
	if err != nil {
		status, state = http.StatusServiceUnavailable, "unavailable"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Status  string `json:"status"`
		Version string `json:"version"`
		Schema  int    `json:"schema"`
	}{Status: state, Version: version.Version, Schema: schema})
}
