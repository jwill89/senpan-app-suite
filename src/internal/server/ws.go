package server

import "net/http"

// handleWS upgrades an HTTP connection to a WebSocket and registers it with the hub.
// If id is provided, the connection is associated with that card (player);
// otherwise it joins the privileged admin channel and must be authenticated.
//
//	Endpoint:  GET /api/ws[?id=XXXXXX]
//	Auth:      player connections (id present) are public; admin connections
//	           (no id) require an authenticated, active account
//	Params:    id (optional) — card ID for player connections
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	cardID := r.URL.Query().Get("id")
	// Admin connections (no card id) join the channel that streams draws
	// immediately — bypassing the player draw-delay — plus winner card IDs, so
	// they must come from a logged-in account. Without this gate anyone could
	// open the admin channel and peek (defeating the draw-delay anti-cheat).
	// The /api/ws route bypasses the session middleware, so load it manually.
	if cardID == "" && s.wsSessionUser(r) == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized – login required")
		return
	}
	s.hub.ServeWS(w, r, cardID)
}
