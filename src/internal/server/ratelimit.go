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

// clientIP extracts the IP address from the request, checking X-Forwarded-For
// for the real client IP behind a reverse proxy, then falling back to RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can be "client, proxy1, proxy2" — take the first.
		if i := strings.Index(xff, ","); i > 0 {
			xff = xff[:i]
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
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
