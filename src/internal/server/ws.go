package server

import "net/http"

// handleWS upgrades an HTTP connection to a WebSocket and registers it with the hub.
// If id is provided, the connection is associated with that card (player).
//
//	Endpoint:  GET /api/ws[?id=XXXXXX]
//	Auth:      public (bypasses session middleware)
//	Params:    id (optional) — card ID for player connections
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	cardID := r.URL.Query().Get("id")
	s.hub.ServeWS(w, r, cardID)
}
