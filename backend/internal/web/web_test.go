package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// A build as Vite lays it out: the page, a hashed bundle, the service worker with its
// registration, Workbox, the manifest and a sound.
var build = fstest.MapFS{
	"index.html":             {Data: []byte(`<!doctype html><div id="app"></div><script src="/assets/index-abc123.js"></script>`)},
	"assets/index-abc123.js": {Data: []byte("console.log(1)")},
	"sw.js":                  {Data: []byte("self.skipWaiting()")},
	"registerSW.js":          {Data: []byte("navigator.serviceWorker")},
	"workbox-dcde9eb3.js":    {Data: []byte("workbox")},
	"manifest.webmanifest":   {Data: []byte(`{"name":"Senpan"}`)},
	"sounds/queue_pop.mp3":   {Data: []byte("ID3")},
}

func get(t *testing.T, h http.Handler, method, target string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	for k, v := range header {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func newHandler(t *testing.T) *Handler {
	t.Helper()
	h, err := New(build)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestPagesServeTheAppNeverCached(t *testing.T) {
	h := newHandler(t)
	for _, target := range []string{"/", "/admin/bingo", "/raffles/12", "/index.html"} {
		w := get(t, h, http.MethodGet, target, nil)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `id="app"`) {
			t.Fatalf("%s: %d %q, want the app's page", target, w.Code, w.Body.String())
		}
		if got := w.Header().Get("Cache-Control"); got != noStore {
			t.Fatalf("%s: Cache-Control %q, want %q", target, got, noStore)
		}
		if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
			t.Fatalf("%s: Content-Type %q", target, got)
		}
	}
}

func TestFilesCarryTheirTypeAndCache(t *testing.T) {
	h := newHandler(t)
	cases := []struct{ target, contentType, cache string }{
		{"/assets/index-abc123.js", "text/javascript", immutable},
		{"/workbox-dcde9eb3.js", "text/javascript", immutable},
		{"/sw.js", "text/javascript", noStore},
		{"/registerSW.js", "text/javascript", noStore},
		{"/manifest.webmanifest", "application/manifest+json", revalidate},
		{"/sounds/queue_pop.mp3", "audio/mpeg", revalidate},
	}
	for _, c := range cases {
		w := get(t, h, http.MethodGet, c.target, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d", c.target, w.Code)
		}
		if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, c.contentType) {
			t.Fatalf("%s: Content-Type %q, want %q", c.target, got, c.contentType)
		}
		if got := w.Header().Get("Cache-Control"); got != c.cache {
			t.Fatalf("%s: Cache-Control %q, want %q", c.target, got, c.cache)
		}
	}
}

func TestEveryResponseCarriesTheSecurityHeaders(t *testing.T) {
	h := newHandler(t)
	for _, target := range []string{"/", "/assets/index-abc123.js"} {
		w := get(t, h, http.MethodGet, target, nil)
		for _, name := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Permissions-Policy", "Strict-Transport-Security"} {
			if w.Header().Get(name) == "" {
				t.Fatalf("%s: no %s", target, name)
			}
		}
		if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Fatalf("%s: CSP %q", target, csp)
		}
	}
}

// A missing file is a 404, never the app's page served under a script's or image's name.
func TestMissingFilesAndOtherFoldersAre404(t *testing.T) {
	h := newHandler(t)
	for _, target := range []string{
		"/assets/gone-999.js", "/favicon.ico", "/sounds/none.mp3",
		"/api", "/api/no-such-route",
		"/images/logo.png", "/images", "/fonts/x.woff2", "/carrd/site/", "/plugin/pluginmaster.json",
	} {
		if w := get(t, h, http.MethodGet, target, nil); w.Code != http.StatusNotFound {
			t.Fatalf("%s: %d, want 404", target, w.Code)
		}
	}
	// A page whose name only starts like a folder is still a page.
	if w := get(t, h, http.MethodGet, "/imagesettings", nil); w.Code != http.StatusOK {
		t.Fatalf("/imagesettings: %d, want the app's page", w.Code)
	}
}

func TestARevalidationIsAnswered304(t *testing.T) {
	h := newHandler(t)
	first := get(t, h, http.MethodGet, "/manifest.webmanifest", nil)
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}
	if w := get(t, h, http.MethodGet, "/manifest.webmanifest", map[string]string{"If-None-Match": etag}); w.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match: %d, want 304", w.Code)
	}
}

func TestOnlyGetAndHeadAreAnswered(t *testing.T) {
	h := newHandler(t)
	if w := get(t, h, http.MethodHead, "/", nil); w.Code != http.StatusOK || w.Body.Len() != 0 {
		t.Fatalf("HEAD /: %d, %d bytes", w.Code, w.Body.Len())
	}
	w := get(t, h, http.MethodPost, "/", nil)
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST /: %d, Allow %q", w.Code, w.Header().Get("Allow"))
	}
}

func TestABuildWithoutItsPageIsRefused(t *testing.T) {
	if _, err := New(fstest.MapFS{"assets/a.js": {Data: []byte("x")}}); err == nil {
		t.Fatal("a build without index.html was accepted")
	}
}

// The binary's own build has its page, so a release never ships without the front end.
func TestTheEmbeddedBuildHasItsPage(t *testing.T) {
	if _, err := New(Dist()); err != nil {
		t.Fatal(err)
	}
}
