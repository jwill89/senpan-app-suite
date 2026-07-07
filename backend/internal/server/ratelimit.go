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

// clientIP extracts the client IP for rate-limiting, reading X-Forwarded-For
// behind the reverse proxy and falling back to RemoteAddr.
//
// Security: it takes the RIGHTMOST X-Forwarded-For entry, not the leftmost. The
// deployment sits behind a single trusted reverse proxy (Apache) that appends
// the real client IP to the right of the header; everything to its left is
// supplied by the client and therefore spoofable. Taking the leftmost value (as
// this previously did) let an attacker send "X-Forwarded-For: <random>" to
// appear as a new IP on every request and bypass the per-IP login throttle.
// (If the topology ever grows to multiple trusted hops or a CDN, switch to a
// trusted-proxy-count/range strategy such as realclientip-go.)
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
			return last
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
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
