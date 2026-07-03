package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"app-suite/internal/model"
	"app-suite/internal/server"
)

// fontEnv builds a test env whose webRoot/fonts dir is pre-seeded with the given
// font files, and returns the env. The admin is logged in (needed to manage
// font metadata); the public font endpoints themselves need no auth.
func fontEnv(t *testing.T, files map[string][]byte) *testEnv {
	t.Helper()
	webRoot := t.TempDir()
	fontsDir := filepath.Join(webRoot, "fonts")
	if err := os.MkdirAll(fontsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(fontsDir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	env := newTestEnvWithWebRoot(t, webRoot)
	env.loginAdmin(t)
	return env
}

// getWithHeaders issues a GET with extra request headers.
func (e *testEnv) getWithHeaders(t *testing.T, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, e.url(path), nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// setFontOrigins saves one font's per-font origin allowlist and asserts success.
func (e *testEnv) setFontOrigins(t *testing.T, base string, origins ...string) {
	t.Helper()
	resp := e.patchJSON(t, "/api/fonts/families/"+base, map[string]any{"origins": origins})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("PATCH /api/fonts/families/%s = %d: %s", base, resp.StatusCode, body)
	}
}

// kitCSS fetches kit.css with optional headers and returns its body.
func (e *testEnv) kitCSS(t *testing.T, headers map[string]string) string {
	t.Helper()
	resp := e.getWithHeaders(t, "/api/fonts/pub/kit.css", headers)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET kit.css = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

// kitToken extracts the serving token for the (single) font in kit.css.
func (e *testEnv) kitToken(t *testing.T) string {
	t.Helper()
	m := regexp.MustCompile(`url\('f/([^']+)'\)`).FindStringSubmatch(e.kitCSS(t, nil))
	if m == nil {
		t.Fatal("no tokenized src in kit.css")
	}
	return m[1]
}

func TestFontServe_KitCSS(t *testing.T) {
	// Fake bytes: conversion fails at startup, so the TTF itself is served.
	env := fontEnv(t, map[string][]byte{"My Font.ttf": []byte("ttf-bytes")})

	css := env.kitCSS(t, nil)
	if !strings.Contains(css, "font-family:'My Font'") {
		t.Errorf("kit.css missing family rule:\n%s", css)
	}
	if strings.Contains(css, "My Font.ttf") || strings.Contains(css, "My%20Font") {
		t.Errorf("kit.css leaks the real filename:\n%s", css)
	}
	if !strings.Contains(css, "format('truetype')") {
		t.Errorf("kit.css missing format hint:\n%s", css)
	}
}

func TestFontServe_KitFilteredByReferer(t *testing.T) {
	env := fontEnv(t, map[string][]byte{"A.woff2": []byte("woff2"), "B.woff2": []byte("woff2")})
	env.setFontOrigins(t, "A", "https://mysite.carrd.co")

	// The allowed site sees only its font; an unknown site sees none;
	// same-host (and Referer-less) requests see everything.
	allowed := env.kitCSS(t, map[string]string{"Referer": "https://mysite.carrd.co/about"})
	if !strings.Contains(allowed, "font-family:'A'") || strings.Contains(allowed, "font-family:'B'") {
		t.Errorf("allowed-site kit should contain only font A:\n%s", allowed)
	}
	foreign := env.kitCSS(t, map[string]string{"Referer": "https://evil.example.com/page"})
	if strings.Contains(foreign, "@font-face") {
		t.Errorf("foreign-site kit should contain no fonts:\n%s", foreign)
	}
	local := env.kitCSS(t, map[string]string{"Referer": env.url("/some/page")})
	if !strings.Contains(local, "font-family:'A'") || !strings.Contains(local, "font-family:'B'") {
		t.Errorf("same-host kit should contain every font:\n%s", local)
	}
}

func TestFontServe_OriginGate(t *testing.T) {
	env := fontEnv(t, map[string][]byte{"Body.woff2": []byte("woff2-bytes")})
	env.setFontOrigins(t, "Body", "https://mysite.carrd.co")
	token := env.kitToken(t)
	path := "/api/fonts/pub/f/" + token

	t.Run("no origin at all is refused", func(t *testing.T) {
		resp := env.getWithHeaders(t, path, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("bare request = %d; want 403", resp.StatusCode)
		}
	})

	t.Run("allowlisted origin is served with CORS echo", func(t *testing.T) {
		resp := env.getWithHeaders(t, path, map[string]string{"Origin": "https://mysite.carrd.co"})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("allowed origin = %d; want 200", resp.StatusCode)
		}
		if acao := resp.Header.Get("Access-Control-Allow-Origin"); acao != "https://mysite.carrd.co" {
			t.Errorf("ACAO = %q; want the requesting origin", acao)
		}
		if vary := resp.Header.Get("Vary"); !strings.Contains(vary, "Origin") {
			t.Errorf("Vary = %q; want Origin", vary)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "font/woff2" {
			t.Errorf("Content-Type = %q; want font/woff2", ct)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "woff2-bytes" {
			t.Errorf("body = %q; want the font bytes", body)
		}
	})

	t.Run("non-listed origin is refused", func(t *testing.T) {
		resp := env.getWithHeaders(t, path, map[string]string{"Origin": "https://other.carrd.co"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("foreign origin = %d; want 403", resp.StatusCode)
		}
	})

	t.Run("allowlists are per font", func(t *testing.T) {
		// A second font with no origins: the same site must NOT get it.
		fontsDir := filepath.Join(env.srv.WebRootForTest(), "fonts")
		if err := os.WriteFile(filepath.Join(fontsDir, "Locked.woff2"), []byte("locked"), 0644); err != nil {
			t.Fatal(err)
		}
		var lockedToken string
		for _, f := range env.listFonts(t).Fonts {
			if f.Base == "Locked" {
				lockedToken = f.ServedToken
			}
		}
		if lockedToken == "" {
			t.Fatal("Locked font not listed")
		}
		resp := env.getWithHeaders(t, "/api/fonts/pub/f/"+lockedToken,
			map[string]string{"Origin": "https://mysite.carrd.co"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("other font with same origin = %d; want 403 (allowlists are per font)", resp.StatusCode)
		}
	})

	t.Run("same-origin requests always pass", func(t *testing.T) {
		hostOrigin := strings.TrimSuffix(env.url(""), "/")
		for name, headers := range map[string]map[string]string{
			"origin":         {"Origin": hostOrigin},
			"sec-fetch-site": {"Sec-Fetch-Site": "same-origin"},
			"referer":        {"Referer": env.url("/admin")},
		} {
			resp := env.getWithHeaders(t, path, headers)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("same-origin via %s = %d; want 200", name, resp.StatusCode)
			}
		}
	})

	t.Run("unknown token 404s", func(t *testing.T) {
		resp := env.getWithHeaders(t, "/api/fonts/pub/f/AAAAAAAAAAAAAAAAAAAAAA.woff2",
			map[string]string{"Sec-Fetch-Site": "same-origin"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("bogus token = %d; want 404", resp.StatusCode)
		}
	})
}

func TestFontServe_TokenExpiry(t *testing.T) {
	env := fontEnv(t, map[string][]byte{"Old.ttf": []byte("ttf")})
	same := map[string]string{"Sec-Fetch-Site": "same-origin"}
	bucket := server.FontTokenBucketForTest(time.Now())

	// Previous-bucket tokens still serve (grace window)…
	prev := server.FontFileTokenForTest(env.srv, "Old.ttf", bucket-1)
	resp := env.getWithHeaders(t, "/api/fonts/pub/f/"+prev, same)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("previous-bucket token = %d; want 200", resp.StatusCode)
	}

	// …older ones have expired.
	stale := server.FontFileTokenForTest(env.srv, "Old.ttf", bucket-2)
	resp = env.getWithHeaders(t, "/api/fonts/pub/f/"+stale, same)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("two-buckets-old token = %d; want 404", resp.StatusCode)
	}
}

func TestFontFamilies_OriginsValidation(t *testing.T) {
	env := fontEnv(t, map[string][]byte{"F.otf": []byte("otf")})

	t.Run("normalizes and dedupes", func(t *testing.T) {
		env.setFontOrigins(t, "F", "https://MySite.Carrd.co/", "https://mysite.carrd.co", " https://other.example.com ")
		got := env.listFonts(t).Fonts[0].Origins
		want := []string{"https://mysite.carrd.co", "https://other.example.com"}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("saved origins = %v; want %v", got, want)
		}
	})

	t.Run("rejects non-origin entries", func(t *testing.T) {
		for _, bad := range []string{"mysite.carrd.co", "https://site.com/path", "ftp://site.com", "https://"} {
			resp := env.patchJSON(t, "/api/fonts/families/F", map[string]any{"origins": []string{bad}})
			resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("origin %q = %d; want 400", bad, resp.StatusCode)
			}
		}
	})

	t.Run("requires the fonts permission", func(t *testing.T) {
		fresh := newTestEnvWithWebRoot(t, t.TempDir()) // not logged in
		resp := fresh.patchJSON(t, "/api/fonts/families/F", map[string]any{"origins": []string{}})
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("unauthenticated PATCH = %d; want 401", resp.StatusCode)
		}
	})

	t.Run("unknown font 404s", func(t *testing.T) {
		resp := env.patchJSON(t, "/api/fonts/families/Nope", map[string]any{"origins": []string{}})
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("unknown font = %d; want 404", resp.StatusCode)
		}
	})
}

func TestFontServe_SettingsPayloadCarriesTokens(t *testing.T) {
	env := fontEnv(t, map[string][]byte{"Header.woff": []byte("woff")})
	resp := env.get(t, "/api/settings")
	defer resp.Body.Close()
	var out model.SettingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.UploadedFonts) != 1 {
		t.Fatalf("uploaded_fonts = %+v; want one entry", out.UploadedFonts)
	}
	f := out.UploadedFonts[0]
	// The fake bytes can't convert, so the uploaded WOFF itself is served.
	if f.Name != "Header" || f.Family != "Header" || !strings.HasSuffix(f.Token, ".woff") {
		t.Errorf("uploaded font = %+v; want base name + tokenized .woff link", f)
	}
	// The settings token must actually serve the file same-origin.
	resp2 := env.getWithHeaders(t, "/api/fonts/pub/f/"+f.Token,
		map[string]string{"Sec-Fetch-Site": "same-origin"})
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("settings token fetch = %d; want 200", resp2.StatusCode)
	}
}

func TestNormalizeFontOrigin(t *testing.T) {
	valid := map[string]string{
		"https://mysite.carrd.co":   "https://mysite.carrd.co",
		"https://MySite.Carrd.co/":  "https://mysite.carrd.co",
		"http://localhost:5173":     "http://localhost:5173",
		"  https://a.example.com ":  "https://a.example.com",
		"HTTPS://UPPER.example.com": "https://upper.example.com",
	}
	for in, want := range valid {
		got, ok := server.NormalizeFontOriginForTest(in)
		if !ok || got != want {
			t.Errorf("normalizeFontOrigin(%q) = (%q, %v); want (%q, true)", in, got, ok, want)
		}
	}
	invalid := []string{
		"", "mysite.carrd.co", "https://site.com/path", "https://site.com?q=1",
		"https://site.com#f", "ftp://site.com", "https://user:pw@site.com", "https://",
	}
	for _, in := range invalid {
		if got, ok := server.NormalizeFontOriginForTest(in); ok {
			t.Errorf("normalizeFontOrigin(%q) = (%q, true); want ok=false", in, got)
		}
	}
}
