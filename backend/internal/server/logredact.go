package server

import (
	"net/url"
	"strings"
)

// Capability tokens travel in the URL path (and, for the PAT fallback, the query
// string): /api/garapon/{token}, /api/stamp-card/{token}, /api/fonts/pub/f/{token},
// and the SPA referer forms /garapon/{token}, /stamp-card/{token}. Those tokens
// are bearer credentials, so the request logger must not write them verbatim to
// the rotating file, the /api/logs viewer, or the admin WebSocket tail. The
// trailing slash on each prefix keeps the plural admin routes (/api/garapons/{id},
// integer ids) from ever matching.
var tokenPathPrefixes = []string{
	"/api/garapon/",
	"/api/stamp-card/",
	"/api/fonts/pub/f/",
	"/garapon/",
	"/stamp-card/",
}

// redactSensitivePath replaces a capability-token path segment with a short,
// non-reversible hash (tok_ + 8 hex of SHA-256). Requests for the same link
// still correlate in the logs — needed for draw-abuse / stamp-fraud
// investigation — while the raw replayable secret never appears. Any trailing
// segment (/draw, /stamp) is preserved. Non-token paths are returned unchanged.
func redactSensitivePath(path string) string {
	for _, pre := range tokenPathPrefixes {
		if !strings.HasPrefix(path, pre) {
			continue
		}
		rest := path[len(pre):]
		tok, tail := rest, ""
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			tok, tail = rest[:i], rest[i:]
		}
		if tok == "" {
			return path
		}
		// 8 hex chars (32 bits) of the token's SHA-256 — a stable, non-reversible
		// correlation key (hashToken returns the full hex digest; see tokens.go).
		return pre + "tok_" + hashToken(tok)[:8] + tail
	}
	return path
}

// redactTokenQuery replaces the value of a `token` query parameter (the PAT /
// capability-token fallback accepted on some routes) with a fixed placeholder.
// Other parameters are left intact. Returns raw unchanged when there is no token.
func redactTokenQuery(raw string) string {
	if raw == "" {
		return ""
	}
	vals, err := url.ParseQuery(raw)
	if err != nil {
		return raw
	}
	if _, ok := vals["token"]; !ok {
		return raw
	}
	vals.Set("token", "REDACTED")
	return vals.Encode()
}

// redactReferer scrubs a capability token out of a Referer URL (its path or
// query), so the DEBUG request-detail line can't leak a token that a browser
// carried from a /stamp-card/{token} or /garapon/{token} page.
func redactReferer(ref string) string {
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	u.Path = redactSensitivePath(u.Path)
	u.RawQuery = redactTokenQuery(u.RawQuery)
	return u.String()
}
