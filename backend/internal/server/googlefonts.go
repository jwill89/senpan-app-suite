package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"app-suite/internal/model"
)

// The Google Fonts family list, for pages that let a visitor pick a font (the Ema
// plaque designer). The API key is a secret setting, so the browser can't fetch
// the list itself: the server does, with its own key, and hands out the names.

// googleFontsTTL is how long a fetched list is served before it is fetched again.
// New families appear on Google's side every few weeks, not hours.
const googleFontsTTL = 12 * time.Hour

// googleFontsRetry is how long a failed fetch is remembered, so a bad key or an
// outage costs one outbound call per window instead of one per visitor.
const googleFontsRetry = 5 * time.Minute

// googleFontsCache holds the last fetched list and the key it was fetched with
// (changing the key in Settings refetches).
type googleFontsCache struct {
	mu        sync.Mutex
	key       string
	families  []string
	fetchedAt time.Time
	failed    bool
}

// handleGoogleFonts returns the Google Fonts family names, most popular first.
//
//	Endpoint:  GET /api/google-fonts
//	Auth:      public
//	Response:  200 GoogleFontsResponse (empty when no API key is configured)
func (s *Server) handleGoogleFonts(w http.ResponseWriter, r *http.Request) {
	key, err := s.store.GetSetting(r.Context(), "google_fonts_api_key")
	if err != nil {
		writeInternalError(w, "read google fonts key", err)
		return
	}
	writeJSON(w, http.StatusOK, model.GoogleFontsResponse{Families: s.googleFontFamilies(r.Context(), strings.TrimSpace(key))})
}

// googleFontFamilies serves the cached list, fetching it when it is stale, missing
// or was fetched under a different key. Never nil.
func (s *Server) googleFontFamilies(ctx context.Context, key string) []string {
	c := &s.googleFonts
	c.mu.Lock()
	defer c.mu.Unlock()
	if key == "" {
		return []string{}
	}
	ttl := googleFontsTTL
	if c.failed {
		ttl = googleFontsRetry
	}
	if c.key == key && time.Since(c.fetchedAt) < ttl {
		return c.families
	}
	// Detached: the list is cached for every later visitor, so one who leaves
	// mid-fetch shouldn't throw the fetch away.
	families, err := fetchGoogleFonts(context.WithoutCancel(ctx), key)
	c.key, c.fetchedAt, c.failed = key, time.Now(), err != nil
	if err != nil {
		slog.Warn("fetch google fonts list", "error", err)
		c.families = []string{}
		return c.families
	}
	c.families = families
	return families
}

// fetchGoogleFonts reads the family list from the Google Fonts Developer API.
func fetchGoogleFonts(ctx context.Context, key string) ([]string, error) {
	u := "https://www.googleapis.com/webfonts/v1/webfonts?sort=popularity&key=" + url.QueryEscape(key)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // a fixed Google API host; only the key varies
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }() // decoded in full below
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google fonts api answered %d", resp.StatusCode)
	}
	var body struct {
		Items []struct {
			Family string `json:"family"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	families := make([]string, 0, len(body.Items))
	for _, it := range body.Items {
		if it.Family != "" {
			families = append(families, it.Family)
		}
	}
	return families, nil
}
