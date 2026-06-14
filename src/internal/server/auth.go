package server

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
)

// authRequest is the JSON body for POST /api/auth.
// Action is "login" (default) or "logout".
type authRequest struct {
	Action   string `json:"action"`
	Password string `json:"password"`
}

// handleAuthCheck returns the current admin authentication status.
//
//	Endpoint:  GET /api/auth
//	Auth:      public
//	Response:  {"authenticated": bool}
func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": s.isAdmin(r),
	})
}

// handleAuthAction processes login and logout requests.
// Login uses constant-time comparison to prevent timing attacks.
// Rate-limited by IP to prevent brute-force attacks.
//
//	Endpoint:  POST /api/auth
//	Auth:      public
//	Request:   {"action": "login"|"logout", "password": "..."}
//	Response:  {"success": true} or {"error": "..."}
func (s *Server) handleAuthAction(w http.ResponseWriter, r *http.Request) {
	req, err := readJSON[authRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Action == "" {
		req.Action = "login"
	}

	if req.Action == "logout" {
		s.sessions.Put(r.Context(), "is_admin", false)
		_ = s.sessions.Destroy(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}

	// Rate limit check for login attempts.
	ip := clientIP(r)
	if s.limiter.isLimited(ip) {
		slog.Warn("auth rate limited", "ip", ip)
		writeError(w, http.StatusTooManyRequests, "Too many failed attempts. Please try again later.")
		return
	}

	// Login — constant-time comparison to prevent timing attacks.
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.password)) == 1 {
		// Rotate session token on privilege escalation to prevent session fixation.
		_ = s.sessions.RenewToken(r.Context())
		s.sessions.Put(r.Context(), "is_admin", true)
		s.limiter.resetFailures(ip)
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}

	s.limiter.recordFailure(ip)
	slog.Warn("auth failed", "ip", ip)
	writeError(w, http.StatusUnauthorized, "Invalid password")
}
