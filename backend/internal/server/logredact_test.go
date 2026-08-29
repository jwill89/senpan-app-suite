package server

import (
	"strings"
	"testing"
)

func TestRedactSensitivePath(t *testing.T) {
	// Admin / non-token paths must be left completely untouched.
	unchanged := []string{
		"/api/garapons/5",            // plural admin route, integer id
		"/api/garapons/5/players",    // plural admin sub-route
		"/api/stamp-rallies/12/logs", // plural admin route
		"/api/settings",              // ordinary route
		"/api/fonts/pub/kit.css",     // font kit (not a token file)
		"/",                          // root
	}
	for _, p := range unchanged {
		if got := redactSensitivePath(p); got != p {
			t.Errorf("redactSensitivePath(%q) = %q; want unchanged", p, got)
		}
	}

	// Token routes: the token segment is replaced, any trailing action is kept,
	// and the same token always maps to the same placeholder (correlatable).
	drawA := redactSensitivePath("/api/garapon/SECRET123/draw")
	if drawA == "/api/garapon/SECRET123/draw" {
		t.Fatal("garapon token was not redacted")
	}
	if want := "/api/garapon/tok_"; drawA[:len(want)] != want {
		t.Errorf("redacted path = %q; want prefix %q", drawA, want)
	}
	if drawA[len(drawA)-5:] != "/draw" {
		t.Errorf("redacted path = %q; want trailing /draw preserved", drawA)
	}
	if redactSensitivePath("/api/garapon/SECRET123") == "/api/garapon/SECRET123" {
		t.Error("garapon token (no action) was not redacted")
	}
	// Stable correlation: same token -> same placeholder.
	if redactSensitivePath("/api/garapon/SECRET123/draw") != drawA {
		t.Error("redaction is not stable for the same token")
	}
	// Different token -> different placeholder.
	if redactSensitivePath("/api/garapon/OTHER/draw") == drawA {
		t.Error("different tokens collided to the same placeholder")
	}

	for _, p := range []string{
		"/api/stamp-card/ABC/stamp",
		"/api/fonts/pub/f/XYZ",
		"/stamp-card/DEF", // SPA referer form
		"/garapon/GHI",    // SPA referer form
	} {
		if redactSensitivePath(p) == p {
			t.Errorf("redactSensitivePath(%q) left the token unredacted", p)
		}
	}
}

func TestRedactTokenQuery(t *testing.T) {
	if got := redactTokenQuery("token=snp_secret&x=1"); got == "token=snp_secret&x=1" {
		t.Errorf("token query not redacted: %q", got)
	}
	if got := redactTokenQuery("a=1&b=2"); got != "a=1&b=2" {
		t.Errorf("non-token query changed: %q", got)
	}
	if got := redactTokenQuery(""); got != "" {
		t.Errorf("empty query = %q; want empty", got)
	}
}

func TestRedactReferer(t *testing.T) {
	if got := redactReferer("https://apps.example/stamp-card/SECRET"); got == "https://apps.example/stamp-card/SECRET" {
		t.Errorf("referer token not redacted: %q", got)
	}
	if got := redactReferer("https://apps.example/admin/themes"); got != "https://apps.example/admin/themes" {
		t.Errorf("non-token referer changed: %q", got)
	}
}

// TestRedactSensitivePathIsCaseInsensitive pins the deliberate case-insensitive
// prefix match. Every other case in this file uses an all-lowercase route prefix,
// so removing the strings.ToLower - which the function's own comment spends five
// lines justifying - was an undetected mutation. The routes really do match any
// casing on the server, so a token link requested as /API/Garapon/... reaches the
// handler and must not then be written to the log verbatim.
func TestRedactSensitivePathIsCaseInsensitive(t *testing.T) {
	const token = "abcdef0123456789abcdef0123456789"
	cases := []string{
		"/API/Garapon/" + token,
		"/Api/Stamp-Card/" + token + "/stamp",
		"/GARAPON/" + token + "/draw",
		"/api/Fonts/Pub/F/" + token,
	}
	for _, path := range cases {
		got := redactSensitivePath(path)
		if strings.Contains(got, token) {
			t.Errorf("redactSensitivePath(%q) = %q; the raw token survived a differently-cased prefix", path, got)
		}
		if !strings.Contains(got, "tok_") {
			t.Errorf("redactSensitivePath(%q) = %q; want the tok_ correlation hash", path, got)
		}
	}

	// The prefix's ORIGINAL casing is preserved, and a trailing segment survives.
	if got := redactSensitivePath("/API/Garapon/" + token + "/draw"); !strings.HasPrefix(got, "/API/Garapon/") ||
		!strings.HasSuffix(got, "/draw") {
		t.Errorf("redaction rewrote the request's own casing or dropped its tail: %q", got)
	}
}
