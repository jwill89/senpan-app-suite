package anilist_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"app-suite/internal/anilist"
)

// rewrite sends every request to a fixed test server, whatever its address.
type rewrite struct{ target *url.URL }

func (rt rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme, req.URL.Host = rt.target.Scheme, rt.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func clientFor(t *testing.T, h http.HandlerFunc) *anilist.Client {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	target, _ := url.Parse(ts.URL)
	return &anilist.Client{HTTP: &http.Client{Transport: rewrite{target}, Timeout: time.Second}}
}

const endpoint = "https://graphql.anilist.co"

func TestSearchMapsAFullTitle(t *testing.T) {
	var method, contentType string
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		method, contentType = r.Method, r.Header.Get("Content-Type")
		_, _ = io.WriteString(w, `{"data":{"Page":{"media":[{
			"id":111621,
			"title":{"romaji":"Yahwacheop","english":"Painter of the Night","native":"야화첩"},
			"description":"Na-kyum is a talented artist.<br><br>(Source: Seven Seas)",
			"coverImage":{"extraLarge":"https://s4.anilist.co/cover.jpg","large":"https://s4.anilist.co/med.jpg"},
			"format":"MANGA","countryOfOrigin":"KR","genres":["Drama","Romance"],
			"chapters":131,"siteUrl":"https://anilist.co/manga/111621"}]}}}`)
	})

	got, err := c.Search(t.Context(), endpoint, "painter")
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || contentType != "application/json" {
		t.Errorf("upstream request = %s %q; want a JSON POST", method, contentType)
	}
	if len(got) != 1 {
		t.Fatalf("results = %d; want 1", len(got))
	}
	r := got[0]
	if r.Title != "Painter of the Night" || r.CoverImage != "https://s4.anilist.co/cover.jpg" ||
		r.Format != "Manhwa" || r.Genres != "Drama, Romance" || r.Chapters != "131" {
		t.Errorf("suggestion = %+v", r)
	}
	if strings.Contains(r.Summary, "<br>") || !strings.Contains(r.Summary, "Na-kyum is a talented artist.") {
		t.Errorf("summary not stripped cleanly: %q", r.Summary)
	}
	if len(r.Sources) != 1 || r.Sources[0].Title != "AniList" || r.Sources[0].URL != "https://anilist.co/manga/111621" {
		t.Errorf("sources = %+v; want the AniList link", r.Sources)
	}
}

func TestByID(t *testing.T) {
	c := clientFor(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"id":7`) {
			_, _ = io.WriteString(w, `{"data":{"Media":{"id":7,"title":{"romaji":"Nanatsu"},"format":"MANGA","countryOfOrigin":"JP"}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"Media":null}}`)
	})
	got, err := c.ByID(t.Context(), endpoint, 7)
	if err != nil || len(got) != 1 || got[0].Title != "Nanatsu" || got[0].Format != "Manga" {
		t.Fatalf("ByID(7) = %+v, %v", got, err)
	}
	got, err = c.ByID(t.Context(), endpoint, 8)
	if err != nil || len(got) != 0 {
		t.Errorf("ByID(8) = %+v, %v; want none and no error", got, err)
	}
}

func lookupError(t *testing.T, h http.HandlerFunc) *anilist.Error {
	t.Helper()
	_, err := clientFor(t, h).Search(t.Context(), endpoint, "x")
	var ae *anilist.Error
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v; want an *anilist.Error", err)
	}
	return ae
}

func TestErrorsAreClassified(t *testing.T) {
	// A 403 with AniList's own reason is a refusal, carrying that reason.
	e := lookupError(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"errors":[{"message":"AniList API temporarily disabled"}]}`)
	})
	if e.Kind != anilist.Refused || e.Status != 403 || e.Message != "AniList API temporarily disabled" {
		t.Errorf("403 = %+v", e)
	}
	if status, _ := anilist.HTTPStatus(e); status != http.StatusInternalServerError {
		t.Errorf("a refusal answers %d; want 500", status)
	}

	// A 429 is a rate limit that says how long to wait.
	e = lookupError(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "42")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	if e.Kind != anilist.RateLimited || e.RetryAfter != 42*time.Second {
		t.Errorf("429 = %+v", e)
	}
	if status, wait := anilist.HTTPStatus(e); status != http.StatusTooManyRequests || wait != 42*time.Second {
		t.Errorf("a rate limit answers %d after %v", status, wait)
	}

	// A 5xx is AniList being unreachable.
	e = lookupError(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) })
	if e.Kind != anilist.Unreachable {
		t.Errorf("502 = %+v", e)
	}
	if status, _ := anilist.HTTPStatus(e); status != http.StatusServiceUnavailable {
		t.Errorf("unreachable answers %d; want 503", status)
	}

	// Junk with a 200 is unreachable too; an error in the GraphQL answer is a refusal.
	e = lookupError(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "<html>") })
	if e.Kind != anilist.Unreachable {
		t.Errorf("junk = %+v", e)
	}
	e = lookupError(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"errors":[{"message":"Bad query"}]}`)
	})
	if e.Kind != anilist.Refused || e.Message != "Bad query" {
		t.Errorf("graphql error = %+v", e)
	}
}

func TestNeverAnswersWithAnUpstreamStatus(t *testing.T) {
	for _, err := range []error{
		&anilist.Error{Kind: anilist.Refused, Status: 502},
		&anilist.Error{Kind: anilist.Unreachable, Status: 424},
		errors.New("plain"),
	} {
		status, _ := anilist.HTTPStatus(err)
		if status == http.StatusBadGateway || status == http.StatusFailedDependency {
			t.Errorf("HTTPStatus(%v) = %d", err, status)
		}
	}
}

func TestAnAbandonedLookupStops(t *testing.T) {
	c := clientFor(t, func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.Search(ctx, endpoint, "x"); err == nil {
		t.Error("a cancelled lookup should fail")
	}
}

func TestEndpointFallsBackToTheDefault(t *testing.T) {
	cases := map[string]string{
		"":                                       "https://graphql.anilist.co",
		"  ":                                     "https://graphql.anilist.co",
		"https://graphql.anilist.co/":            "https://graphql.anilist.co",
		"https://staging.anilist.co/graphql":     "https://staging.anilist.co/graphql",
		"http://graphql.anilist.co":              "https://graphql.anilist.co",
		"https://evil.example/graphql":           "https://graphql.anilist.co",
		"https://anilist.co.evil.example":        "https://graphql.anilist.co",
		"https://169.254.169.254/latest":         "https://graphql.anilist.co",
		"https://graphql.anilist.co.evil.test/x": "https://graphql.anilist.co",
	}
	for in, want := range cases {
		if got := anilist.Endpoint(in); got != want {
			t.Errorf("Endpoint(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestRedirectsAreCheckedAgainstTheAllowlist(t *testing.T) {
	c := anilist.New()
	allowed, _ := http.NewRequest(http.MethodGet, "https://graphql.anilist.co/x", nil)
	if err := c.HTTP.CheckRedirect(allowed, []*http.Request{{}}); err != nil {
		t.Errorf("an AniList redirect was blocked: %v", err)
	}
	internal, _ := http.NewRequest(http.MethodGet, "http://169.254.169.254/latest", nil)
	if err := c.HTTP.CheckRedirect(internal, []*http.Request{{}}); err == nil {
		t.Error("a redirect to a metadata address was followed")
	}
	if err := c.HTTP.CheckRedirect(allowed, make([]*http.Request, 5)); err == nil {
		t.Error("a sixth hop should stop")
	}
}

func TestDeriveFormat(t *testing.T) {
	cases := []struct{ format, country, want string }{
		{"MANGA", "JP", "Manga"},
		{"MANGA", "KR", "Manhwa"},
		{"MANGA", "CN", "Manhua"},
		{"NOVEL", "CN", "Danmei"},
		{"NOVEL", "JP", "Novel"},
		{"ONE_SHOT", "JP", "One-shot"},
		{"MANGA", "", "Manga"},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := anilist.DeriveFormat(c.format, c.country); got != c.want {
			t.Errorf("DeriveFormat(%q,%q) = %q; want %q", c.format, c.country, got, c.want)
		}
	}
}

func TestStripHTML(t *testing.T) {
	got := anilist.StripHTML("One<br>Two<br/><br><br><br>Three <i>very</i> &amp; <b>bold</b>")
	if got != "One\nTwo\n\nThree very & bold" {
		t.Errorf("StripHTML = %q", got)
	}
}

func TestToSuggestionFallbacks(t *testing.T) {
	var m anilist.Media
	m.Title.Native = "ネイティブ"
	m.CoverImage.Large = "https://x/large.jpg"
	zero := 0
	m.Chapters = &zero
	s := anilist.ToSuggestion(m)
	if s.Title != "ネイティブ" || s.CoverImage != "https://x/large.jpg" || s.Chapters != "" || len(s.Sources) != 0 {
		t.Errorf("suggestion = %+v", s)
	}
}
