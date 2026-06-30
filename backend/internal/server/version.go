package server

import (
	"net/http"

	"app-suite/internal/version"
)

// handleVersion reports the backend's semantic version. Public (no auth): the
// value isn't sensitive — the SPA pairs it with its own build version in the
// admin dashboard for a compatibility check — and a public endpoint also doubles
// as a lightweight health/version probe.
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Backend string `json:"backend"`
	}{Backend: version.Version})
}
