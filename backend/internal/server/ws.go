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
	var revalidate func() bool
	if cardID == "" {
		user := s.wsSessionUser(r)
		if user == nil {
			writeError(w, http.StatusUnauthorized, "Unauthorized – login required")
			return
		}
		// The connection is only authorized at accept time; re-check the account
		// periodically (see hub writePump) so an account deactivated or deleted
		// mid-session has its admin socket dropped instead of streaming forever.
		// Uses a context-free lookup by id — the request context is gone once the
		// upgrade handler returns. Catches deactivation/deletion (the anti-peek
		// threat: staff revoked during a live game); a browser logout closes its
		// own socket client-side, and a revoked plugin token is rejected on the
		// plugin's next reconnect.
		id := user.ID
		revalidate = func() bool {
			u, err := s.store.GetUserByID(id)
			return err == nil && u != nil && u.IsActive
		}
	}
	s.hub.ServeWS(w, r, cardID, revalidate)
}
