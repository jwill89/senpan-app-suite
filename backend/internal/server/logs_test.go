package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"app-suite/internal/server"
)

func TestLogClientIP_PrefersCloudflareHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	r.RemoteAddr = "[::1]:44686"
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 172.71.1.1")
	r.Header.Set("CF-Connecting-IP", "198.51.100.7")
	// CF-Connecting-IP wins over XFF and RemoteAddr.
	if got := server.LogClientIPForTest(r); got != "198.51.100.7" {
		t.Errorf("with CF-Connecting-IP: got %q; want 198.51.100.7", got)
	}
	// Without it, the leftmost XFF (original client) is used.
	r.Header.Del("CF-Connecting-IP")
	if got := server.LogClientIPForTest(r); got != "203.0.113.9" {
		t.Errorf("XFF leftmost: got %q; want 203.0.113.9", got)
	}
	// With neither, the RemoteAddr host (port stripped).
	r.Header.Del("X-Forwarded-For")
	if got := server.LogClientIPForTest(r); got != "::1" {
		t.Errorf("RemoteAddr host: got %q; want ::1", got)
	}
}

func TestLogClientIP_IgnoresForgedHeadersFromNonLoopback(t *testing.T) {
	// A client that reaches the backend directly (RemoteAddr is NOT the loopback
	// proxy) must not be able to forge its logged IP via CF-Connecting-IP / XFF.
	r := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	r.RemoteAddr = "1.2.3.4:5555"
	r.Header.Set("CF-Connecting-IP", "9.9.9.9")
	r.Header.Set("X-Forwarded-For", "8.8.8.8")
	if got := server.LogClientIPForTest(r); got != "1.2.3.4" {
		t.Errorf("forged headers from non-loopback peer: got %q; want 1.2.3.4 (RemoteAddr)", got)
	}

	// From the trusted loopback proxy, the headers ARE honored.
	r.RemoteAddr = "127.0.0.1:5555"
	if got := server.LogClientIPForTest(r); got != "9.9.9.9" {
		t.Errorf("from loopback proxy: got %q; want 9.9.9.9 (CF-Connecting-IP)", got)
	}
}
