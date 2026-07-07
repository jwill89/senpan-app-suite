package server

import (
	"net/http"
	"strings"
)

// actorCtxKey is the context key carrying the per-request actor holder.
type actorCtxKey struct{}

// requestActor identifies who made a request, for the access log. It is created
// in ServeHTTP — which logs OUTSIDE the session middleware and so can't resolve
// the session itself — and populated by withActor INSIDE the handler chain,
// where the loaded session, the per-request user cache, and any auth token are
// available. The pointer is shared through the request context, so ServeHTTP
// reads back what withActor filled in.
type requestActor struct {
	user string // account username, empty when unauthenticated
	kind string // "session" | "token" | "bot" | "anon"
	bot  string // verified-bot category or User-Agent, when kind == "bot"
}

// withActor resolves the request's actor once, early in the handler chain, and
// stashes it on the shared holder for the access log. It sits INSIDE
// LoadAndSave + withUserCache so the session and cache are ready; resolving here
// also warms the user cache the handler's permission guards reuse, so it adds no
// extra store reads for authenticated requests (and none for anonymous ones).
func (s *Server) withActor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a, ok := r.Context().Value(actorCtxKey{}).(*requestActor); ok {
			s.fillActor(r, a)
		}
		next.ServeHTTP(w, r)
	})
}

// fillActor classifies the request: an authenticated account (cookie session or
// PAT — currentUser resolves both), a Cloudflare-verified good bot, or plain
// anonymous traffic.
func (s *Server) fillActor(r *http.Request, a *requestActor) {
	if u := s.currentUser(r); u != nil {
		a.user = u.Username
		if s.sessionUserID(r) != 0 {
			a.kind = "session"
		} else {
			a.kind = "token" // no cookie session → authenticated by PAT (the plugin)
		}
		return
	}
	if name, ok := verifiedBot(r); ok {
		a.kind = "bot"
		a.bot = name
		return
	}
	a.kind = "anon"
}

// verifiedBot reports whether Cloudflare has verified the request as a known-good
// bot, and a human-readable name for it. Cloudflare delivers the signal to the
// origin one of two ways:
//
//   - cf-verified-bot (+ cf-verified-bot-category): the "Add bot protection
//     headers" managed transform — Enterprise plan with Bot Management; or
//   - x-verified-bot: a custom Request Header Transform Rule on the cf.client.bot
//     field, which works on any plan (Free/Pro included).
//
// The bot's own User-Agent is used as the name when Cloudflare sends no category,
// and that UA is trustworthy here precisely because Cloudflare already verified
// the source — it is no longer a spoofable self-claim.
//
// This is a logging hint only, never a security decision. Like CF-Connecting-IP
// it is forgeable by a client that reaches the origin without passing through
// Cloudflare; lock the origin to Cloudflare's IP ranges (or Authenticated Origin
// Pulls) if you need it to be tamper-proof.
func verifiedBot(r *http.Request) (string, bool) {
	if !truthyHeader(r, "Cf-Verified-Bot") && !truthyHeader(r, "X-Verified-Bot") {
		return "", false
	}
	if cat := strings.TrimSpace(r.Header.Get("Cf-Verified-Bot-Category")); cat != "" {
		return cat, true
	}
	if ua := strings.TrimSpace(r.UserAgent()); ua != "" {
		return ua, true
	}
	return "verified", true
}

// truthyHeader reports whether a header is present with a true-ish value.
func truthyHeader(r *http.Request, name string) bool {
	switch strings.ToLower(strings.TrimSpace(r.Header.Get(name))) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}
