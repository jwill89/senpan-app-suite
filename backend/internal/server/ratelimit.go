package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// rateLimiter tracks failed attempts per IP for brute-force protection.
type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attemptInfo
	maxFails int
	window   time.Duration
	stop     chan struct{}
}

// attemptInfo tracks the number of failed login attempts from a single IP
// within a sliding time window.
type attemptInfo struct {
	count       int
	windowStart time.Time
}

// newRateLimiter creates a rate limiter that allows maxFails failed attempts
// per IP within the given window duration. Starts a background goroutine
// that periodically purges expired entries. Call close(rl.stop) to terminate.
func newRateLimiter(maxFails int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		attempts: make(map[string]*attemptInfo),
		maxFails: maxFails,
		window:   window,
		stop:     make(chan struct{}),
	}
	// Periodically clean up expired entries.
	go func() {
		ticker := time.NewTicker(window)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rl.cleanup()
			case <-rl.stop:
				return
			}
		}
	}()
	return rl
}

// clientIP returns the authoritative client IP used as the per-IP key for the
// login / passkey / registration (and public raffle / card-request) rate
// limiters. It delegates to logClientIP so the throttle key and the IP the access
// log records are always the SAME value.
//
// This deliberately reuses logClientIP's loopback-gated trust rather than the
// naive "rightmost X-Forwarded-For" it previously used. Behind Cloudflare→Apache
// the header at the backend is "<real client>, <cloudflare edge>", so the
// rightmost entry is the Cloudflare EDGE IP — meaning every user behind the same
// edge shared one throttle bucket (one victim tripping the limit, or an attacker
// rotating edges, poisoned everyone else). logClientIP resolves the true origin
// (CF-Connecting-IP, then leftmost XFF) but ONLY when the immediate peer is the
// loopback reverse proxy; a direct/off-proxy peer falls back to the spoof-proof
// RemoteAddr host, so the header can't be forged to mint a fresh key per request.
func clientIP(r *http.Request) string {
	return logClientIP(r)
}

// logClientIP returns the best-guess real client IP for LOGGING/display only
// (never for security decisions). Behind Cloudflare the authoritative client
// address is CF-Connecting-IP (set by Cloudflare, overriding any client-supplied
// value); fall back to the leftmost X-Forwarded-For entry (the original origin),
// then the RemoteAddr host. This favors the true origin address for
// human-readable logs, unlike clientIP which takes the spoof-resistant rightmost
// entry for rate-limiting.
//
// Security: the proxy-supplied headers are honored ONLY when the immediate peer
// (RemoteAddr) is the local reverse proxy — i.e. a loopback address, since Apache
// ProxyPasses to localhost:8080. A client that can reach the backend directly (or
// craft the header) could otherwise forge CF-Connecting-IP / X-Forwarded-For and
// poison the audit log's `ip` field. If the proxy is ever bound off-loopback,
// widen this trusted set (see clientIP's note).
func logClientIP(r *http.Request) string {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if cf := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cf != "" {
			return cf
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
				return first
			}
		}
	}
	return host
}

// isLimited returns true if the given IP has exceeded the failure limit.
func (rl *rateLimiter) isLimited(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	info, ok := rl.attempts[ip]
	if !ok {
		return false
	}
	if time.Since(info.windowStart) > rl.window {
		delete(rl.attempts, ip)
		return false
	}
	return info.count >= rl.maxFails
}

// recordFailure increments the failure count for an IP.
func (rl *rateLimiter) recordFailure(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	info, ok := rl.attempts[ip]
	if !ok || time.Since(info.windowStart) > rl.window {
		rl.attempts[ip] = &attemptInfo{count: 1, windowStart: time.Now()}
		return
	}
	info.count++
}

// resetFailures clears the failure count for an IP (on successful login).
func (rl *rateLimiter) resetFailures(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, ip)
}

// cleanup removes expired entries.
func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	for ip, info := range rl.attempts {
		if now.Sub(info.windowStart) > rl.window {
			delete(rl.attempts, ip)
		}
	}
}
