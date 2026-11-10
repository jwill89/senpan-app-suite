package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// problem reads a problem details answer, failing the test if it isn't one.
func problem(t *testing.T, resp *http.Response) (title string, status int, detail string) {
	t.Helper()
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("content type %q, want application/problem+json", ct)
	}
	var p struct {
		Title  string `json:"title"`
		Status int    `json:"status"`
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	return p.Title, p.Status, p.Detail
}

// A path the API doesn't have answers problem details, 404, and a method a path
// doesn't take answers 405 with the methods it does, never the mux's plain text.
func TestAPIUnknownPathsAnswerProblemDetails(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/v1/no-such-thing")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown path: %d, want 404", resp.StatusCode)
	}
	if _, status, detail := problem(t, resp); status != http.StatusNotFound || detail == "" {
		t.Errorf("unknown path: status %d, detail %q", status, detail)
	}

	resp = env.del(t, "/api/v1/version")
	if resp.StatusCode != http.StatusMethodNotAllowed || !strings.Contains(resp.Header.Get("Allow"), "GET") {
		t.Fatalf("DELETE /api/v1/version: %d, Allow %q; want 405 naming GET", resp.StatusCode, resp.Header.Get("Allow"))
	}
	if _, status, _ := problem(t, resp); status != http.StatusMethodNotAllowed {
		t.Errorf("405's problem says %d", status)
	}
}

// A trailing slash answers as the path without it.
func TestAPITrailingSlashAnswersAsWithout(t *testing.T) {
	env := newTestEnv(t)
	resp := env.get(t, "/api/v1/version/")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/version/: %d, want 200", resp.StatusCode)
	}
}

// A public read revalidates: no-cache with an ETag, and a request that already holds
// that ETag gets 304 with no body. Anything else is private, no-store.
func TestAPIPublicReadsRevalidate(t *testing.T) {
	env := newTestEnv(t)

	resp := env.get(t, "/api/v1/version")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	etag := resp.Header.Get("ETag")
	if resp.Header.Get("Cache-Control") != "no-cache" || etag == "" || len(body) == 0 {
		t.Fatalf("public read: Cache-Control %q, ETag %q, %d bytes", resp.Header.Get("Cache-Control"), etag, len(body))
	}

	req, _ := http.NewRequest(http.MethodGet, env.url("/api/v1/version"), nil)
	req.Header.Set("If-None-Match", etag)
	again, err := env.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	rest, _ := io.ReadAll(again.Body)
	again.Body.Close()
	if again.StatusCode != http.StatusNotModified || len(rest) != 0 || again.Header.Get("ETag") != etag {
		t.Fatalf("revalidation: %d with %d bytes, ETag %q; want 304, empty, %q", again.StatusCode, len(rest), again.Header.Get("ETag"), etag)
	}

	miss := env.get(t, "/api/v1/no-such-thing")
	miss.Body.Close()
	if cc := miss.Header.Get("Cache-Control"); cc != "private, no-store" {
		t.Errorf("an error's Cache-Control %q, want private, no-store", cc)
	}
}
