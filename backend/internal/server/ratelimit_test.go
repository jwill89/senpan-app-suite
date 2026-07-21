package server

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientIP(t *testing.T) {
	// clientIP now delegates to logClientIP: proxy-supplied headers are trusted
	// ONLY when the immediate peer (RemoteAddr) is the loopback reverse proxy, and
	// the true origin is the leftmost XFF / CF-Connecting-IP — not the rightmost
	// (Cloudflare edge) entry. An off-loopback peer falls back to RemoteAddr host.
	cases := []struct {
		name   string
		xff    string
		cf     string
		remote string
		want   string
	}{
		// Behind the loopback proxy: trust the origin-most values.
		{"loopback trusts leftmost xff", "1.2.3.4, 5.6.7.8, 9.9.9.9", "", "127.0.0.1:5555", "1.2.3.4"},
		{"loopback trims space", " 1.2.3.4 , 5.6.7.8 ", "", "127.0.0.1:5555", "1.2.3.4"},
		{"loopback prefers cf-connecting-ip", "1.2.3.4, 9.9.9.9", "8.8.8.8", "127.0.0.1:5555", "8.8.8.8"},
		{"loopback no headers falls back to host", "", "", "127.0.0.1:5555", "127.0.0.1"},
		// Off-loopback peer: headers are untrusted (spoofable), use RemoteAddr host.
		{"non-loopback ignores xff", "1.2.3.4", "8.8.8.8", "9.9.9.9:5555", "9.9.9.9"},
		{"remoteaddr without port falls back", "", "", "9.9.9.9", "9.9.9.9"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = c.remote
			if c.xff != "" {
				r.Header.Set("X-Forwarded-For", c.xff)
			}
			if c.cf != "" {
				r.Header.Set("CF-Connecting-IP", c.cf)
			}
			if got := clientIP(r); got != c.want {
				t.Fatalf("clientIP = %q, want %q", got, c.want)
			}
		})
	}
}

func TestRateLimiterThreshold(t *testing.T) {
	rl := newRateLimiter(3, time.Hour)
	defer close(rl.stop)
	const ip = "1.2.3.4"

	if rl.isLimited(ip) {
		t.Fatal("a fresh IP must not be limited")
	}
	rl.recordFailure(ip)
	rl.recordFailure(ip)
	if rl.isLimited(ip) {
		t.Fatal("2 failures (< 3) must not be limited")
	}
	rl.recordFailure(ip) // count == 3 == maxFails
	if !rl.isLimited(ip) {
		t.Fatal("3 failures (>= maxFails) must be limited")
	}

	// A successful login clears the IP.
	rl.resetFailures(ip)
	if rl.isLimited(ip) {
		t.Fatal("after resetFailures the IP must not be limited")
	}
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	// Construct directly (no background goroutine) so we can age the window.
	rl := &rateLimiter{
		attempts: map[string]*attemptInfo{},
		maxFails: 1,
		window:   50 * time.Millisecond,
	}
	const ip = "5.6.7.8"

	rl.recordFailure(ip)
	if !rl.isLimited(ip) {
		t.Fatal("1 failure (>= maxFails 1) must be limited")
	}

	// Age the window past expiry; isLimited should both report unlimited and
	// purge the stale entry.
	rl.attempts[ip].windowStart = time.Now().Add(-time.Hour)
	if rl.isLimited(ip) {
		t.Fatal("an expired window must not be limited")
	}
	if _, ok := rl.attempts[ip]; ok {
		t.Fatal("isLimited must delete the expired entry")
	}
}

func TestRateLimiterRecordAfterWindowResets(t *testing.T) {
	rl := &rateLimiter{
		attempts: map[string]*attemptInfo{},
		maxFails: 2,
		window:   50 * time.Millisecond,
	}
	const ip = "7.7.7.7"
	rl.recordFailure(ip)
	rl.recordFailure(ip)
	if !rl.isLimited(ip) {
		t.Fatal("2 >= 2 should be limited")
	}
	// Expire, then a new failure should start a fresh window at count 1.
	rl.attempts[ip].windowStart = time.Now().Add(-time.Hour)
	rl.recordFailure(ip)
	if rl.attempts[ip].count != 1 {
		t.Fatalf("expected count reset to 1 after window expiry, got %d", rl.attempts[ip].count)
	}
}
