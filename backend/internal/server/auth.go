package server

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"app-suite/internal/auth"
	"app-suite/internal/model"
	"app-suite/internal/store"
)

// reservedUsername is the bootstrap super-admin account. It is seeded by the
// users migration and may not be created via registration. It is also protected
// from deletion/deactivation/demotion and from password changes by anyone other
// than itself (see users.go).
const reservedUsername = "admin"

// minPasswordLen is the minimum length enforced for registration and password
// changes. (The seeded "admin"/"admin" bootstrap bypasses this and is meant to
// be rotated to a strong password immediately.)
const minPasswordLen = 8

// authRequest is the JSON body for POST /api/auth.
// Action is "login" (default) or "logout".
type authRequest struct {
	Action         string `json:"action"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	TurnstileToken string `json:"turnstile_token"` // Cloudflare Turnstile token (when enabled)
}

// handleAuthCheck returns the current authentication status and, when logged in,
// the active user (username, admin flag, page permissions) so the frontend can
// gate its UI. The password hash is never part of model.User, so nothing
// sensitive is exposed here.
//
//	Endpoint:  GET /api/auth
//	Auth:      public
//	Response:  {"authenticated": bool, "user": User|null}
func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	writeJSON(w, http.StatusOK, model.AuthCheckResponse{
		Authenticated: u != nil,
		User:          u,
	})
}

// handleAuthAction processes login and logout requests.
// Login verifies the argon2id password hash and is rate-limited by IP to
// prevent brute-force attacks. Inactive accounts are rejected until an admin
// activates them.
//
//	Endpoint:  POST /api/auth
//	Auth:      public
//	Request:   {"action": "login"|"logout", "username": "...", "password": "..."}
//	Response:  {"success": true, "user": User} or {"error": "..."}
func (s *Server) handleAuthAction(w http.ResponseWriter, r *http.Request) {
	req, err := readJSON[authRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Action == "" {
		req.Action = "login"
	}

	if req.Action == "logout" {
		_ = s.sessions.Destroy(r.Context())
		writeJSON(w, http.StatusOK, model.LogoutResponse{Success: true})
		return
	}

	// Rate limit check for login attempts.
	ip := clientIP(r)
	if s.limiter.isLimited(ip) {
		slog.Warn("auth rate limited", "ip", ip)
		writeError(w, http.StatusTooManyRequests, "Too many failed attempts. Please try again later.")
		return
	}

	// Bot check: when Cloudflare Turnstile is configured, a valid one-time token
	// is required before we do any credential work, so automated brute-force
	// clients can't reach the password check. A failed challenge isn't counted
	// against the login limiter — a legit user with an expired token shouldn't be
	// locked out of password attempts; the token is just re-issued on retry.
	if s.turnstileEnabled() && !s.verifyTurnstile(r.Context(), req.TurnstileToken, ip) {
		slog.Warn("turnstile verification failed", "ip", ip)
		writeError(w, http.StatusForbidden, "Bot verification failed. Please complete the challenge and try again.")
		return
	}

	username := strings.TrimSpace(req.Username)
	user, hash, err := s.store.GetUserByUsername(username)
	if err != nil {
		writeInternalError(w, "lookup user", err)
		return
	}

	// Verify credentials. A missing user and a bad password return the same
	// generic message so usernames can't be enumerated via the login form.
	valid := false
	if user != nil {
		if ok, vErr := auth.Verify(req.Password, hash); vErr == nil {
			valid = ok
		}
	} else {
		// No such user: run a dummy verify so this branch costs the same argon2
		// work as a real account, so usernames can't be enumerated by timing.
		auth.DummyVerify(req.Password)
	}
	if !valid {
		s.limiter.recordFailure(ip)
		slog.Warn("auth failed", "ip", ip, "username", username)
		writeError(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}

	if !user.IsActive {
		// Don't count this against the rate limiter — the credentials were correct.
		writeError(w, http.StatusForbidden, "Account pending activation by an administrator")
		return
	}

	// Rotate session token on privilege escalation to prevent session fixation.
	_ = s.sessions.RenewToken(r.Context())
	s.sessions.Put(r.Context(), "user_id", user.ID)
	s.limiter.resetFailures(ip)
	slog.Debug("login succeeded", "user_id", user.ID, "username", username, "admin", user.IsAdmin, "ip", ip)
	// Stamp the last-login time (best-effort — don't fail the login if it errors).
	// The returned user still carries the *previous* value, which is the intended
	// "last seen" semantic; the users table reloads to show the fresh time.
	if err := s.store.UpdateLastLogin(user.ID); err != nil {
		slog.Error("update last login", "error", err, "user_id", user.ID)
	}
	writeJSON(w, http.StatusOK, model.LoginResponse{Success: true, User: *user})
}

// registerRequest is the JSON body for POST /api/register.
type registerRequest struct {
	Username       string `json:"username"`
	Password       string `json:"password"`
	TurnstileToken string `json:"turnstile_token"` // Cloudflare Turnstile token (when enabled)
}

// handleRegister creates a new account from the hidden registration page. The
// account is created inactive (and non-admin, no permissions); an admin must
// activate it before the user can log in. There is no link to this endpoint in
// the UI — admins share the /admin/register URL directly.
//
//	Endpoint:  POST /api/register
//	Auth:      public
//	Request:   {"username": "...", "password": "..."}
//	Response:  {"success": true, "message": "..."}
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	// Registration is public, so throttle it per IP to prevent mass creation of
	// inactive accounts. Every attempt (success or not) counts against the budget.
	ip := clientIP(r)
	if s.regLimiter.isLimited(ip) {
		slog.Warn("register rate limited", "ip", ip)
		writeError(w, http.StatusTooManyRequests, "Too many sign-up attempts. Please try again later.")
		return
	}
	s.regLimiter.recordFailure(ip)

	req, err := readJSON[registerRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Bot check: when Turnstile is configured, require a valid token before
	// creating an account (blocks automated mass-signup). Skipped when disabled.
	if s.turnstileEnabled() && !s.verifyTurnstile(r.Context(), req.TurnstileToken, ip) {
		slog.Warn("turnstile verification failed (register)", "ip", ip)
		writeError(w, http.StatusForbidden, "Bot verification failed. Please complete the challenge and try again.")
		return
	}

	username := strings.TrimSpace(req.Username)
	if username == "" || len(username) > 32 {
		writeError(w, http.StatusBadRequest, "Username must be 1–32 characters")
		return
	}
	if len(req.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "Password must be at least 8 characters")
		return
	}
	if strings.EqualFold(username, reservedUsername) {
		writeError(w, http.StatusBadRequest, "That username is not available")
		return
	}

	hash, err := auth.Hash(req.Password)
	if err != nil {
		writeInternalError(w, "hash password", err)
		return
	}
	if _, err := s.store.CreateUser(username, hash); err != nil {
		if errors.Is(err, store.ErrUsernameTaken) {
			writeError(w, http.StatusConflict, "That username is already taken")
			return
		}
		writeInternalError(w, "create user", err)
		return
	}

	slog.Info("user registered (pending activation)", "username", username)
	writeJSON(w, http.StatusOK, model.RegisterResponse{
		Success: true,
		Message: "Account created. An administrator must activate it before you can log in.",
	})
}
