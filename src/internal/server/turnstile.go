package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Cloudflare Turnstile is a privacy-friendly "are you a robot?" check. The login
// form renders a widget (using the public site key) that produces a one-time
// token; this server verifies that token with Cloudflare before authenticating,
// so automated brute-force clients can't reach the password check.

// turnstileVerifyURL is Cloudflare's siteverify endpoint. A package var so tests
// can point it at a stub server.
var turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// turnstileHTTPClient is the outbound client for siteverify. Short timeout: this
// sits on the login path, and a slow/unreachable Cloudflare should fail fast.
var turnstileHTTPClient = &http.Client{Timeout: 10 * time.Second}

// turnstileEnabled reports whether the bot check is configured (a secret key is
// set). When false, verification is skipped entirely — keeping local/dev and the
// test harness usable without keys.
func (s *Server) turnstileEnabled() bool {
	return s.turnstileSecret != ""
}

// verifyTurnstile validates a client token against Cloudflare's siteverify API.
// remoteIP is optional (Cloudflare cross-checks it when supplied). Returns true
// only when Cloudflare confirms the token; any error / malformed response / empty
// token is treated as a failure (fail-closed).
func (s *Server) verifyTurnstile(ctx context.Context, token, remoteIP string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	form := url.Values{}
	form.Set("secret", s.turnstileSecret)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, turnstileVerifyURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := turnstileHTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var out struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out); err != nil {
		return false
	}
	return out.Success
}

// handleConfig serves the client bootstrap config. Public — it carries only the
// non-secret Turnstile site key (empty when the bot check is disabled), which the
// login page reads to decide whether to render the challenge.
//
//	Endpoint:  GET /api/config
//	Auth:      public
//	Response:  {"turnstile_site_key": "..."}
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		TurnstileSiteKey string `json:"turnstile_site_key"`
	}{TurnstileSiteKey: s.turnstileSiteKey})
}
