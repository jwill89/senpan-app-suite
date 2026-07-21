package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"app-suite/internal/server"
)

// itoa is a tiny local int64→string helper to keep test call sites terse.
func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// createReadingListIn creates a list in the given club, returning its id.
func createReadingListIn(t *testing.T, e *testEnv, club, title string) int64 {
	t.Helper()
	resp := e.postJSON(t, "/api/book-clubs/"+club+"/reading-lists", map[string]any{"title": title})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create list status = %d; want 201", resp.StatusCode)
	}
	body := decodeBody(t, resp)
	list, _ := body["reading_list"].(map[string]any)
	id, _ := list["id"].(float64)
	if id == 0 {
		t.Fatalf("create list returned no id: %+v", body)
	}
	return int64(id)
}

// createReadingList creates a list in the default (yaoi) club, returning its id.
func createReadingList(t *testing.T, e *testEnv, title string) int64 {
	t.Helper()
	return createReadingListIn(t, e, "yaoi", title)
}

func TestReadingListAndItemHTTPCRUD(t *testing.T) {
	e := newTestEnv(t)

	// Unauthenticated calls are rejected.
	resp := e.get(t, "/api/book-clubs/yaoi/reading-lists")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list status = %d; want 401", resp.StatusCode)
	}
	resp.Body.Close()

	e.loginAdmin(t)
	listID := createReadingList(t, e, "Yaoi Faves")

	// Add an item with sources.
	resp = e.postJSON(t, "/api/book-clubs/yaoi/reading-lists/"+itoa(listID)+"/items", map[string]any{
		"item": map[string]any{
			"title":       "Painter of the Night",
			"summary":     "A period romance.",
			"format":      "Manhwa",
			"genres":      "Romance, Historical",
			"chapters":    "102",
			"comments":    "A classic.",
			"cover_image": "https://media.example.org/p.webp",
			"sources": []map[string]string{
				{"title": "OmegaScans", "url": "https://omegascans.org/series/x"},
				{"title": "", "url": ""}, // blank row should be dropped
			},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create item status = %d; want 201", resp.StatusCode)
	}
	resp.Body.Close()

	// Detail should include the item with one (sanitized) source.
	resp = e.get(t, "/api/book-clubs/yaoi/reading-lists/"+itoa(listID))
	body := decodeBody(t, resp)
	list, _ := body["reading_list"].(map[string]any)
	items, _ := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %d; want 1", len(items))
	}
	item0, _ := items[0].(map[string]any)
	sources, _ := item0["sources"].([]any)
	if len(sources) != 1 {
		t.Fatalf("sources = %d; want 1 (blank dropped)", len(sources))
	}
	itemID := int64(item0["id"].(float64))

	// Update the item (PUT the item resource).
	resp = e.putJSON(t, "/api/book-clubs/yaoi/reading-lists/"+itoa(listID)+"/items/"+itoa(itemID), map[string]any{
		"item": map[string]any{"title": "Painter of the Night (Vol 1)"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update item status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete the item (DELETE the item resource → 204).
	resp = e.del(t, "/api/book-clubs/yaoi/reading-lists/"+itoa(listID)+"/items/"+itoa(itemID))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete item status = %d; want 204", resp.StatusCode)
	}
	resp.Body.Close()

	resp = e.get(t, "/api/book-clubs/yaoi/reading-lists/"+itoa(listID))
	body = decodeBody(t, resp)
	list, _ = body["reading_list"].(map[string]any)
	if items, _ := list["items"].([]any); len(items) != 0 {
		t.Fatalf("items after delete = %d; want 0", len(items))
	}
}

// TestReadingListClubMismatchGuard verifies the security guard: a list created in
// one club cannot be read or mutated through another club's path, even by an
// admin (who holds every club's permission). The mismatch is a 404 — the id is
// not disclosed to the wrong club — while the correct-club path still works.
func TestReadingListClubMismatchGuard(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)

	// A list that belongs to yaoi, plus one item in it.
	listID := createReadingListIn(t, e, "yaoi", "Yaoi Only")
	resp := e.postJSON(t, "/api/book-clubs/yaoi/reading-lists/"+itoa(listID)+"/items", map[string]any{
		"item": map[string]any{"title": "Secret"},
	})
	item := decodeBody(t, resp)["item"].(map[string]any)
	itemID := int64(item["id"].(float64))

	yaoi := "/api/book-clubs/yaoi/reading-lists/" + itoa(listID)
	yuri := "/api/book-clubs/yuri/reading-lists/" + itoa(listID)

	// Reaching the yaoi list through the yuri path 404s on every operation.
	mismatches := []struct {
		name string
		resp *http.Response
	}{
		{"detail", e.get(t, yuri)},
		{"update", e.putJSON(t, yuri, map[string]any{"title": "Hijacked"})},
		{"item create", e.postJSON(t, yuri+"/items", map[string]any{"item": map[string]any{"title": "X"}})},
		{"item update", e.putJSON(t, yuri+"/items/"+itoa(itemID), map[string]any{"item": map[string]any{"title": "X"}})},
		{"item delete", e.del(t, yuri+"/items/"+itoa(itemID))},
		{"publish", e.postJSON(t, yuri+"/publish", map[string]any{})},
		{"delete", e.del(t, yuri)},
	}
	for _, m := range mismatches {
		if m.resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s via yuri path status = %d; want 404 (club mismatch)", m.name, m.resp.StatusCode)
		}
		m.resp.Body.Close()
	}

	// The correct (yaoi) path still resolves the list — the guard didn't touch it.
	resp = e.get(t, yaoi)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("yaoi detail status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestReadingListPerClubPermission verifies the per-club page permission: a
// non-admin who holds only bookclub-yaoi may list/create in yaoi but is
// forbidden (403) from another club's reading lists.
func TestReadingListPerClubPermission(t *testing.T) {
	e := newTestEnv(t)
	yaoiUser := makeActiveUser(t, e, "yaoi-curator", "password123", []string{"bookclub-yaoi"})

	// Granted club: listing and creating work.
	resp := getAs(t, yaoiUser, e, "/api/book-clubs/yaoi/reading-lists")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("yaoi list status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()
	resp = postAs(t, yaoiUser, e, "/api/book-clubs/yaoi/reading-lists", map[string]any{"title": "Mine"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("yaoi create status = %d; want 201", resp.StatusCode)
	}
	resp.Body.Close()

	// Ungranted club: both list and create are forbidden (not just hidden).
	resp = getAs(t, yaoiUser, e, "/api/book-clubs/yuri/reading-lists")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("yuri list status = %d; want 403", resp.StatusCode)
	}
	resp.Body.Close()
	resp = postAs(t, yaoiUser, e, "/api/book-clubs/yuri/reading-lists", map[string]any{"title": "Nope"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("yuri create status = %d; want 403", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDiscordWebhookSettingRedactedForPublic(t *testing.T) {
	e := newTestEnv(t)
	if err := e.store.SetSetting("discord_webhook_url_yaoi", "https://discord.test/secret"); err != nil {
		t.Fatal(err)
	}

	// Public (no login): webhook must be blanked.
	resp := e.get(t, "/api/settings")
	body := decodeBody(t, resp)
	settings, _ := body["settings"].(map[string]any)
	if v, _ := settings["discord_webhook_url_yaoi"].(string); v != "" {
		t.Fatalf("public webhook = %q; want blank", v)
	}

	// Admin: webhook is visible.
	e.loginAdmin(t)
	resp = e.get(t, "/api/settings")
	body = decodeBody(t, resp)
	settings, _ = body["settings"].(map[string]any)
	if v, _ := settings["discord_webhook_url_yaoi"].(string); v != "https://discord.test/secret" {
		t.Fatalf("admin webhook = %q; want the real value", v)
	}
}

func TestPublishPostsOneEmbedPerItem(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)

	// Fake Discord webhook that records each received embed payload.
	var mu sync.Mutex
	var received []map[string]any
	discord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(data, &payload)
		mu.Lock()
		received = append(received, payload)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer discord.Close()
	if err := e.store.SetSetting("discord_webhook_url_yaoi", discord.URL); err != nil {
		t.Fatal(err)
	}

	listID := createReadingList(t, e, "To Publish")
	for _, title := range []string{"Alpha", "Beta", "Gamma"} {
		resp := e.postJSON(t, "/api/book-clubs/yaoi/reading-lists/"+itoa(listID)+"/items", map[string]any{
			"item": map[string]any{"title": title, "format": "Manga", "summary": title + " summary"},
		})
		resp.Body.Close()
	}

	resp := e.postJSON(t, "/api/book-clubs/yaoi/reading-lists/"+itoa(listID)+"/publish", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("publish status = %d; want 200", resp.StatusCode)
	}
	body := decodeBody(t, resp)
	if n, _ := body["published"].(float64); int(n) != 3 {
		t.Fatalf("published = %v; want 3", body["published"])
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 3 {
		t.Fatalf("webhook received %d posts; want 3", len(received))
	}
	// Each payload has exactly one embed with a title.
	for _, p := range received {
		embeds, _ := p["embeds"].([]any)
		if len(embeds) != 1 {
			t.Fatalf("embeds per post = %d; want 1", len(embeds))
		}
		embed, _ := embeds[0].(map[string]any)
		if _, ok := embed["title"].(string); !ok {
			t.Fatalf("embed missing title: %+v", embed)
		}
	}
}

func TestPublishUsesPerClubCommentsLabel(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)

	// Capture the embed posted to Discord for the Yuri club.
	var got map[string]any
	discord := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer discord.Close()
	if err := e.store.SetSetting("discord_webhook_url_yuri", discord.URL); err != nil {
		t.Fatal(err)
	}

	// Create a Yuri list (club from the path), with one commented item.
	listID := createReadingListIn(t, e, "yuri", "Yuri Faves")

	resp := e.postJSON(t, "/api/book-clubs/yuri/reading-lists/"+itoa(listID)+"/items", map[string]any{
		"item": map[string]any{"title": "Bloom Into You", "comments": "Drani loved it."},
	})
	resp.Body.Close()

	resp = e.postJSON(t, "/api/book-clubs/yuri/reading-lists/"+itoa(listID)+"/publish", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("publish status = %d; want 200", resp.StatusCode)
	}
	resp.Body.Close()

	embeds, _ := got["embeds"].([]any)
	if len(embeds) != 1 {
		t.Fatalf("embeds = %d; want 1", len(embeds))
	}
	embed, _ := embeds[0].(map[string]any)
	fields, _ := embed["fields"].([]any)
	found := false
	for _, f := range fields {
		fm, _ := f.(map[string]any)
		if name, _ := fm["name"].(string); name == "Drani's Comments" {
			found = true
		}
		if name, _ := fm["name"].(string); name == "Yao's Comments" {
			t.Errorf("yuri embed used Yao's Comments label")
		}
	}
	if !found {
		t.Errorf("yuri embed missing \"Drani's Comments\" field; fields=%+v", fields)
	}
}

func TestPublishWithoutWebhookFails(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	listID := createReadingList(t, e, "No Hook")
	resp := e.postJSON(t, "/api/book-clubs/yaoi/reading-lists/"+itoa(listID)+"/items", map[string]any{
		"item": map[string]any{"title": "X"},
	})
	resp.Body.Close()

	resp = e.postJSON(t, "/api/book-clubs/yaoi/reading-lists/"+itoa(listID)+"/publish", map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("publish w/o webhook status = %d; want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestBookclubLookupMapsAniListFields(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)

	// Fake AniList GraphQL endpoint returning one manga from a Page search.
	var gotMethod, gotContentType string
	anilist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"Page":{"media":[{
			"id":111621,
			"title":{"romaji":"Yahwacheop","english":"Painter of the Night","native":"야화첩"},
			"description":"Na-kyum is a talented artist.<br><br>(Source: Seven Seas)",
			"coverImage":{"extraLarge":"https://s4.anilist.co/cover.jpg","large":"https://s4.anilist.co/med.jpg"},
			"format":"MANGA","countryOfOrigin":"KR","genres":["Drama","Romance"],
			"chapters":131,"siteUrl":"https://anilist.co/manga/111621"}]}}}`)
	}))
	defer anilist.Close()
	// anilistURL() now enforces the SSRF allowlist at read time, so a localhost
	// endpoint would be rejected and fall back to the real AniList. Keep a valid,
	// allowlisted endpoint URL and route the request to the mock via a host-rewriting
	// transport — exercising the real allowlisted path end to end.
	if err := e.store.SetSetting("anilist_api_url", "https://graphql.anilist.co"); err != nil {
		t.Fatal(err)
	}
	restore, err := server.RouteAnilistToForTest(anilist.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restore)

	resp := e.get(t, "/api/bookclub/lookup?q=painter")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lookup status = %d; want 200", resp.StatusCode)
	}
	// The proxy must POST GraphQL as JSON.
	if gotMethod != http.MethodPost {
		t.Errorf("upstream method = %s; want POST", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Errorf("upstream Content-Type = %q; want application/json", gotContentType)
	}

	body := decodeBody(t, resp)
	results, _ := body["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results = %d; want 1", len(results))
	}
	r0, _ := results[0].(map[string]any)
	// Prefers the English title.
	if r0["title"] != "Painter of the Night" {
		t.Errorf("title = %v", r0["title"])
	}
	// extraLarge cover preferred.
	if r0["cover_image"] != "https://s4.anilist.co/cover.jpg" {
		t.Errorf("cover_image = %v", r0["cover_image"])
	}
	// MANGA + KR → Manhwa.
	if r0["format"] != "Manhwa" {
		t.Errorf("format = %v; want Manhwa", r0["format"])
	}
	if r0["genres"] != "Drama, Romance" {
		t.Errorf("genres = %v", r0["genres"])
	}
	if r0["chapters"] != "131" {
		t.Errorf("chapters = %v", r0["chapters"])
	}
	// HTML stripped from the summary (<br> → newline, no tags left).
	s, _ := r0["summary"].(string)
	if strings.Contains(s, "<br>") || !strings.Contains(s, "Na-kyum is a talented artist.") {
		t.Errorf("summary not stripped cleanly: %q", s)
	}
	// AniList source prefilled.
	sources, _ := r0["sources"].([]any)
	if len(sources) != 1 {
		t.Fatalf("sources = %d; want 1", len(sources))
	}
	src0, _ := sources[0].(map[string]any)
	if src0["title"] != "AniList" || src0["url"] != "https://anilist.co/manga/111621" {
		t.Errorf("source = %v; want AniList link", src0)
	}
}

func TestDeriveFormatByCountry(t *testing.T) {
	cases := []struct {
		format, country, want string
	}{
		{"MANGA", "JP", "Manga"},
		{"MANGA", "KR", "Manhwa"},
		{"MANGA", "CN", "Manhua"},
		{"NOVEL", "CN", "Danmei"},
		{"NOVEL", "JP", "Novel"},
		{"ONE_SHOT", "JP", "One-shot"},
		{"MANGA", "", "Manga"},
	}
	for _, c := range cases {
		if got := server.DeriveFormatForTest(c.format, c.country); got != c.want {
			t.Errorf("deriveFormat(%q,%q) = %q; want %q", c.format, c.country, got, c.want)
		}
	}
}
