package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"

	"app-suite/internal/model"
)

// Personal access tokens (PATs) let external API clients — notably the FFXIV
// plugin — authenticate as a user without the browser's cookie-session +
// Turnstile login flow. A client sends `Authorization: Bearer <token>` (or, for
// the WebSocket upgrade, which can't always set headers, a `?token=<token>`
// query param). The token resolves to its owning account and the SAME per-page
// permission guards a logged-in session gets then apply, so a PAT never grants
// more than the user already has.
//
// Format: "pat_" + base64url(32 random bytes) → 256 bits of entropy. That high
// entropy is why a fast SHA-256 (not argon2) is the right at-rest hash: there is
// no low-entropy secret to brute-force, and the lookup stays a single indexed
// query. Only the hash is stored; the plaintext is returned to the user exactly
// once, at generation.

const (
	tokenScheme = "pat_"
	// tokenPrefixLen is how many leading characters of the token are kept in the
	// clear for display/identification in the account UI. Revealing a handful of
	// the token's 43 base64 characters leaves its entropy effectively intact.
	tokenPrefixLen = 12
	tokenRandBytes = 32
)

// generatePAT returns a fresh personal access token: the plaintext (shown to the
// user once), its SHA-256 hash (stored), and a short display prefix.
func generatePAT() (token, hash, prefix string, err error) {
	buf := make([]byte, tokenRandBytes)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", err
	}
	token = tokenScheme + base64.RawURLEncoding.EncodeToString(buf)
	prefix = token
	if len(prefix) > tokenPrefixLen {
		prefix = prefix[:tokenPrefixLen]
	}
	return token, hashToken(token), prefix, nil
}

// hashToken returns the hex-encoded SHA-256 of a token — the form stored in
// user_tokens.token_hash and recomputed to look the token up on each request.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// bearerToken extracts a personal access token from the `Authorization: Bearer
// <token>` header only. Returns "" when absent or not a PAT-shaped value.
//
// The `?token=` query-param fallback is deliberately NOT accepted here: a token
// in the URL leaks into access logs, browser history, and Referer headers. Only
// the WebSocket upgrade (which can't always set headers) may use the query param,
// via wsUpgradeToken — every REST route must carry the Authorization header.
func bearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if rest, ok := strings.CutPrefix(h, "Bearer "); ok {
			if tok := strings.TrimSpace(rest); strings.HasPrefix(tok, tokenScheme) {
				return tok
			}
		}
	}
	return ""
}

// wsUpgradeToken resolves the PAT for the WebSocket upgrade path ONLY. It accepts
// the `Authorization: Bearer` header and, additionally, the `?token=` query param
// — the browser WebSocket API can't set request headers, so the plugin/admin
// channel passes the token in the URL. Do NOT use this for REST routes.
func wsUpgradeToken(r *http.Request) string {
	if tok := bearerToken(r); tok != "" {
		return tok
	}
	if tok := strings.TrimSpace(r.URL.Query().Get("token")); strings.HasPrefix(tok, tokenScheme) {
		return tok
	}
	return ""
}

// userFromToken resolves the active account bearing the request's Authorization
// Bearer PAT (REST routes), or nil when absent/unmatched/inactive.
func (s *Server) userFromToken(r *http.Request) *model.User {
	return s.userForToken(bearerToken(r))
}

// wsUserFromToken resolves the active account for a WebSocket upgrade, honoring
// the `?token=` query fallback in addition to the Authorization header.
func (s *Server) wsUserFromToken(r *http.Request) *model.User {
	return s.userForToken(wsUpgradeToken(r))
}

// userForToken resolves the active account for a raw PAT string, or nil when the
// token is empty, matches nothing, or the owning account is inactive. On a hit it
// best-effort stamps the token's last-used time (throttled in the store) so the
// account UI can show when the token was last exercised.
func (s *Server) userForToken(tok string) *model.User {
	if tok == "" {
		return nil
	}
	hash := hashToken(tok)
	u, err := s.store.GetUserByTokenHash(hash)
	if err != nil || u == nil {
		return nil
	}
	_ = s.store.TouchUserToken(hash)
	return u
}

// ── Account token self-service endpoints ─────────────────────────────────────

// handleAccountTokenInfo returns the current user's token metadata — never the
// token itself, which is shown only once at generation.
//
//	Endpoint:  GET /api/account/token
//	Auth:      any active user
//	Response:  {"has_token": bool, "prefix": "...", "created_at": "...", "last_used_at": "..."}
func (s *Server) handleAccountTokenInfo(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	info, err := s.store.GetUserTokenInfo(user.ID)
	if err != nil {
		writeInternalError(w, "get token info", err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleAccountTokenGenerate mints (replacing any existing) the current user's
// personal access token. The freshly generated token's plaintext is returned
// exactly once in this response — it is hashed at rest and can never be shown
// again — so the UI must surface it to the user immediately. Generating a new
// token invalidates the previous one.
//
//	Endpoint:  POST /api/account/token
//	Auth:      any active user
//	Response:  {"token": "...", "prefix": "...", "created_at": "..."}
func (s *Server) handleAccountTokenGenerate(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	token, hash, prefix, err := generatePAT()
	if err != nil {
		writeInternalError(w, "generate token", err)
		return
	}
	if err := s.store.UpsertUserToken(user.ID, hash, prefix); err != nil {
		writeInternalError(w, "store token", err)
		return
	}
	info, err := s.store.GetUserTokenInfo(user.ID)
	if err != nil {
		writeInternalError(w, "load token info", err)
		return
	}
	writeJSON(w, http.StatusOK, model.AccountTokenGenerateResponse{
		Token:     token,
		Prefix:    prefix,
		CreatedAt: info.CreatedAt,
	})
}

// handleAccountTokenRevoke deletes the current user's personal access token. The
// response carries whether a token row was actually removed, so it returns the
// {"ok": true, "deleted": bool} body (not a bare 204).
//
//	Endpoint:  DELETE /api/account/token
//	Auth:      any active user
//	Response:  200 {"ok": true, "deleted": bool}
func (s *Server) handleAccountTokenRevoke(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	deleted, err := s.store.DeleteUserToken(user.ID)
	if err != nil {
		writeInternalError(w, "revoke token", err)
		return
	}
	writeJSON(w, http.StatusOK, model.TokenRevokeResponse{OK: true, Deleted: deleted})
}
