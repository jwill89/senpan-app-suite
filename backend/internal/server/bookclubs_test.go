package server_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// itoa is a tiny local int64->string helper to keep test call sites terse.
func itoa(n int64) string { return strconv.FormatInt(n, 10) }

const bookClubWebhook = "https://discord.com/api/webhooks/1/book-club-token"

// itemBody is a full reading-list item save with the given title and fields set.
func itemBody(title string, set map[string]any) map[string]any {
	body := map[string]any{
		"coverImage": "", "title": title, "summary": "", "format": "", "genres": "", "tropes": "",
		"chapters": "", "comments": "", "sources": []any{},
	}
	for k, v := range set {
		body[k] = v
	}
	return body
}

// createReadingListIn creates a list in the given club, returning its id.
func createReadingListIn(t *testing.T, e *testEnv, club, title string) int64 {
	t.Helper()
	var list map[string]any
	decodeInto(t, e.postJSON(t, "/api/v1/book-clubs/"+club+"/reading-lists", map[string]any{"title": title}), http.StatusCreated, &list)
	id, _ := list["id"].(float64)
	if id == 0 || list["club"] != club || list["title"] != title {
		t.Fatalf("create list returned %+v", list)
	}
	if items, ok := list["items"].([]any); !ok || len(items) != 0 {
		t.Fatalf("a new list's items = %v; want an empty array", list["items"])
	}
	return int64(id)
}

// createReadingList creates a list in the yaoi club, returning its id.
func createReadingList(t *testing.T, e *testEnv, title string) int64 {
	t.Helper()
	return createReadingListIn(t, e, "yaoi", title)
}

// addItem adds an item to a list and returns it.
func addItem(t *testing.T, e *testEnv, club string, listID int64, title string, set map[string]any) map[string]any {
	t.Helper()
	var item map[string]any
	decodeInto(t, e.postJSON(t, "/api/v1/book-clubs/"+club+"/reading-lists/"+itoa(listID)+"/items", itemBody(title, set)), http.StatusCreated, &item)
	return item
}

func TestReadingListAndItemHTTPCRUD(t *testing.T) {
	e := newTestEnv(t)

	// Unauthenticated calls are rejected.
	if got := status(e.get(t, "/api/v1/book-clubs/yaoi/reading-lists")); got != http.StatusUnauthorized {
		t.Fatalf("anon list status = %d; want 401", got)
	}

	e.loginAdmin(t)
	listID := createReadingList(t, e, "Yaoi Faves")

	// Add an item with sources; a blank row is dropped.
	item := addItem(t, e, "yaoi", listID, "  Painter of the Night  ", map[string]any{
		"summary": "A period romance.", "format": "Manhwa", "genres": "Romance, Historical", "chapters": "102",
		"comments": "A classic.", "coverImage": "https://media.example.org/p.webp",
		"sources": []map[string]string{
			{"title": "OmegaScans", "url": "https://omegascans.org/series/x"},
			{"title": "", "url": ""},
		},
	})
	if item["title"] != "Painter of the Night" || item["listId"] != float64(listID) {
		t.Errorf("created item: %v", item)
	}
	if sources, _ := item["sources"].([]any); len(sources) != 1 {
		t.Fatalf("sources = %d; want 1 (blank dropped)", len(sources))
	}
	itemID := int64(item["id"].(float64))
	itemPath := "/api/v1/book-clubs/yaoi/reading-lists/" + itoa(listID) + "/items/" + itoa(itemID)

	// The list shows it.
	var list map[string]any
	decodeInto(t, e.get(t, "/api/v1/book-clubs/yaoi/reading-lists/"+itoa(listID)), http.StatusOK, &list)
	if items, _ := list["items"].([]any); len(items) != 1 {
		t.Fatalf("items = %d; want 1", len(items))
	}

	// The club's lists, without items.
	var lists struct{ ReadingLists []map[string]any }
	decodeInto(t, e.get(t, "/api/v1/book-clubs/yaoi/reading-lists"), http.StatusOK, &lists)
	if len(lists.ReadingLists) != 1 || lists.ReadingLists[0]["title"] != "Yaoi Faves" {
		t.Fatalf("lists: %v", lists.ReadingLists)
	}
	if _, has := lists.ReadingLists[0]["items"]; has {
		t.Error("the summaries carry items")
	}

	// Replace the item (PUT answers it).
	var updated map[string]any
	decodeInto(t, e.putJSON(t, itemPath, itemBody("Painter of the Night (Vol 1)", nil)), http.StatusOK, &updated)
	if updated["title"] != "Painter of the Night (Vol 1)" || updated["sortOrder"] != item["sortOrder"] {
		t.Errorf("updated: %v", updated)
	}

	// Rename the list (PUT answers it with its items).
	decodeInto(t, e.putJSON(t, "/api/v1/book-clubs/yaoi/reading-lists/"+itoa(listID), map[string]any{"title": "Renamed"}), http.StatusOK, &list)
	if list["title"] != "Renamed" || len(list["items"].([]any)) != 1 {
		t.Errorf("renamed: %v", list)
	}

	// Delete the item, then it is gone.
	if got := status(e.del(t, itemPath)); got != http.StatusNoContent {
		t.Fatalf("delete item status = %d; want 204", got)
	}
	if got := status(e.del(t, itemPath)); got != http.StatusNotFound {
		t.Errorf("delete the item again = %d; want 404", got)
	}
	decodeInto(t, e.get(t, "/api/v1/book-clubs/yaoi/reading-lists/"+itoa(listID)), http.StatusOK, &list)
	if items, _ := list["items"].([]any); len(items) != 0 {
		t.Fatalf("items after delete = %d; want 0", len(items))
	}

	// Delete the list.
	if got := status(e.del(t, "/api/v1/book-clubs/yaoi/reading-lists/"+itoa(listID))); got != http.StatusNoContent {
		t.Errorf("delete list = %d; want 204", got)
	}
	if got := status(e.del(t, "/api/v1/book-clubs/yaoi/reading-lists/"+itoa(listID))); got != http.StatusNotFound {
		t.Errorf("delete the list again = %d; want 404", got)
	}
}

// New items go to the end of their list.
func TestReadingListItemsKeepTheirOrder(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	listID := createReadingList(t, e, "Ordered")
	for _, title := range []string{"First", "Second", "Third"} {
		addItem(t, e, "yaoi", listID, title, nil)
	}
	var list struct{ Items []map[string]any }
	decodeInto(t, e.get(t, "/api/v1/book-clubs/yaoi/reading-lists/"+itoa(listID)), http.StatusOK, &list)
	var titles []string
	for _, it := range list.Items {
		titles = append(titles, it["title"].(string))
	}
	if fmt.Sprint(titles) != "[First Second Third]" {
		t.Errorf("order = %v", titles)
	}
}

func TestReadingListValidation(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	if at := invalid(t, e.postJSON(t, "/api/v1/book-clubs/yaoi/reading-lists", map[string]any{"title": "   "})); !slices.Contains(at, "body.title") {
		t.Errorf("a blank list title: errors at %v", at)
	}
	listID := createReadingList(t, e, "L")
	path := "/api/v1/book-clubs/yaoi/reading-lists/" + itoa(listID)
	if at := invalid(t, e.postJSON(t, path+"/items", itemBody("  ", nil))); !slices.Contains(at, "body.title") {
		t.Errorf("a blank item title: errors at %v", at)
	}
	if got := status(e.postJSON(t, "/api/v1/book-clubs/yaoi/reading-lists/999999/items", itemBody("X", nil))); got != http.StatusNotFound {
		t.Errorf("an item on an unknown list = %d; want 404", got)
	}
	if got := status(e.putJSON(t, path+"/items/999999", itemBody("X", nil))); got != http.StatusNotFound {
		t.Errorf("an unknown item = %d; want 404", got)
	}
}

// TestReadingListItemMustBelongToItsList: the list is checked against the club in the
// path, and the item against the list, so an item id from another club's list finds
// nothing.
func TestReadingListItemMustBelongToItsList(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)

	victimList := createReadingListIn(t, e, "yaoi", "Yaoi Only")
	victimItem := int64(addItem(t, e, "yaoi", victimList, "Secret", nil)["id"].(float64))

	// The attacker's own list, in a different club, reached by its correct path - so
	// the club guard passes and only the item scope stands in the way.
	ownList := createReadingListIn(t, e, "yuri", "Yuri Own")
	own := "/api/v1/book-clubs/yuri/reading-lists/" + itoa(ownList)

	if got := status(e.putJSON(t, own+"/items/"+itoa(victimItem), itemBody("Hijacked", nil))); got != http.StatusNotFound {
		t.Errorf("cross-list item update status = %d; want 404", got)
	}
	if got := status(e.del(t, own+"/items/"+itoa(victimItem))); got != http.StatusNotFound {
		t.Errorf("cross-list item delete status = %d; want 404", got)
	}

	var list struct{ Items []map[string]any }
	decodeInto(t, e.get(t, "/api/v1/book-clubs/yaoi/reading-lists/"+itoa(victimList)), http.StatusOK, &list)
	if len(list.Items) != 1 || list.Items[0]["title"] != "Secret" {
		t.Errorf("the victim list = %v; want its one item untouched", list.Items)
	}
}

// TestReadingListClubMismatchGuard: a list created in one club cannot be read or
// changed through another club's path, even by an admin (who holds every club's
// permission). The mismatch is a 404 - the id is not disclosed to the wrong club -
// while the correct-club path still works.
func TestReadingListClubMismatchGuard(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)

	listID := createReadingListIn(t, e, "yaoi", "Yaoi Only")
	itemID := int64(addItem(t, e, "yaoi", listID, "Secret", nil)["id"].(float64))

	yaoi := "/api/v1/book-clubs/yaoi/reading-lists/" + itoa(listID)
	yuri := "/api/v1/book-clubs/yuri/reading-lists/" + itoa(listID)

	mismatches := []struct {
		name string
		resp *http.Response
	}{
		{"detail", e.get(t, yuri)},
		{"update", e.putJSON(t, yuri, map[string]any{"title": "Hijacked"})},
		{"item create", e.postJSON(t, yuri+"/items", itemBody("X", nil))},
		{"item update", e.putJSON(t, yuri+"/items/"+itoa(itemID), itemBody("X", nil))},
		{"item delete", e.del(t, yuri+"/items/"+itoa(itemID))},
		{"publish", e.postJSON(t, yuri+"/publish", nil)},
		{"delete", e.del(t, yuri)},
	}
	for _, m := range mismatches {
		if got := status(m.resp); got != http.StatusNotFound {
			t.Errorf("%s via yuri path status = %d; want 404 (club mismatch)", m.name, got)
		}
	}

	if got := status(e.get(t, yaoi)); got != http.StatusOK {
		t.Fatalf("yaoi detail status = %d; want 200", got)
	}
}

// TestReadingListPerClubPermission: a non-admin who holds only bookclub-yaoi may
// list and create in yaoi but is forbidden (403) from another club's pages; a club
// that isn't registered is a 404, and signed out is a 401, before either.
func TestReadingListPerClubPermission(t *testing.T) {
	e := newTestEnv(t)
	yaoiUser := makeActiveUser(t, e, "yaoi-curator", "password123", []string{"bookclub-yaoi"})

	if got := status(getAs(t, yaoiUser, e, "/api/v1/book-clubs/yaoi/reading-lists")); got != http.StatusOK {
		t.Fatalf("yaoi list status = %d; want 200", got)
	}
	if got := status(postAs(t, yaoiUser, e, "/api/v1/book-clubs/yaoi/reading-lists", map[string]any{"title": "Mine"})); got != http.StatusCreated {
		t.Fatalf("yaoi create status = %d; want 201", got)
	}

	for _, path := range []string{
		"/api/v1/book-clubs/yuri/reading-lists", "/api/v1/book-clubs/yuri/webhook", "/api/v1/book-clubs/yuri/lookup?q=x",
	} {
		if got := status(getAs(t, yaoiUser, e, path)); got != http.StatusForbidden {
			t.Errorf("GET %s = %d; want 403", path, got)
		}
	}
	if got := status(postAs(t, yaoiUser, e, "/api/v1/book-clubs/yuri/reading-lists", map[string]any{"title": "Nope"})); got != http.StatusForbidden {
		t.Fatalf("yuri create status = %d; want 403", got)
	}
	if got := status(getAs(t, yaoiUser, e, "/api/v1/book-clubs/nope/reading-lists")); got != http.StatusNotFound {
		t.Errorf("an unknown club = %d; want 404", got)
	}

	e.loginAdmin(t)
	if got := status(e.get(t, "/api/v1/book-clubs/nope/reading-lists")); got != http.StatusNotFound {
		t.Errorf("an unknown club, as an admin = %d; want 404", got)
	}
}

// The webhook is the club's page's: read and set there, kept out of the public
// settings and the integrations, and it must be Discord's.
func TestBookClubWebhook(t *testing.T) {
	e := newTestEnv(t)
	if got := status(e.get(t, "/api/v1/book-clubs/yaoi/webhook")); got != http.StatusUnauthorized {
		t.Fatalf("webhook signed out = %d; want 401", got)
	}
	curator := makeActiveUser(t, e, "curator", "password123", []string{"bookclub-yaoi"})
	if got := status(reqAs(t, curator, e, http.MethodPut, "/api/v1/book-clubs/yuri/webhook", map[string]any{"webhookUrl": bookClubWebhook})); got != http.StatusForbidden {
		t.Fatalf("another club's webhook = %d; want 403", got)
	}

	// The club's own curator sets and reads it.
	resp := reqAs(t, curator, e, http.MethodPut, "/api/v1/book-clubs/yaoi/webhook", map[string]any{"webhookUrl": bookClubWebhook})
	var hook map[string]any
	decodeInto(t, resp, http.StatusOK, &hook)
	if hook["webhookUrl"] != bookClubWebhook {
		t.Errorf("set: %v", hook)
	}
	decodeInto(t, getAs(t, curator, e, "/api/v1/book-clubs/yaoi/webhook"), http.StatusOK, &hook)
	if hook["webhookUrl"] != bookClubWebhook {
		t.Errorf("read back: %v", hook)
	}
	// The other club still has none.
	e.loginAdmin(t)
	decodeInto(t, e.get(t, "/api/v1/book-clubs/yuri/webhook"), http.StatusOK, &hook)
	if hook["webhookUrl"] != "" {
		t.Errorf("another club's webhook: %v", hook)
	}

	// It is nowhere else: not the public settings, not the integrations.
	for _, path := range []string{"/api/v1/settings", "/api/v1/settings/integrations"} {
		resp := e.get(t, path)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(body), "book-club-token") {
			t.Errorf("%s carries the webhook: %s", path, body)
		}
	}

	if at := invalid(t, e.putJSON(t, "/api/v1/book-clubs/yaoi/webhook", map[string]any{"webhookUrl": "https://evil.example/api/webhooks/1"})); !slices.Contains(at, "body.webhookUrl") {
		t.Errorf("a non-Discord webhook: errors at %v", at)
	}
	decodeInto(t, e.putJSON(t, "/api/v1/book-clubs/yaoi/webhook", map[string]any{"webhookUrl": ""}), http.StatusOK, &hook)
	if hook["webhookUrl"] != "" {
		t.Errorf("cleared: %v", hook)
	}
}

// useBookClubWebhook saves a club's webhook and points every post at a stub that
// answers with status, recording what it is sent.
func useBookClubWebhook(t *testing.T, e *testEnv, club string, status int, header http.Header) *recordingDiscord {
	t.Helper()
	decodeInto(t, e.putJSON(t, "/api/v1/book-clubs/"+club+"/webhook", map[string]any{"webhookUrl": bookClubWebhook}), http.StatusOK, nil)
	var stub recordingDiscord
	stub.use(t, e, status, header)
	return &stub
}

func TestPublishPostsOneEmbedPerItem(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	discord := useBookClubWebhook(t, e, "yaoi", http.StatusNoContent, nil)

	listID := createReadingList(t, e, "To Publish")
	for _, title := range []string{"Alpha", "Beta", "Gamma"} {
		addItem(t, e, "yaoi", listID, title, map[string]any{"format": "Manga", "summary": title + " summary"})
	}

	var result map[string]any
	decodeInto(t, e.postJSON(t, "/api/v1/book-clubs/yaoi/reading-lists/"+itoa(listID)+"/publish", nil), http.StatusOK, &result)
	if result["published"] != float64(3) {
		t.Fatalf("published = %v; want 3", result["published"])
	}

	if discord.count() != 3 {
		t.Fatalf("webhook received %d posts; want 3", discord.count())
	}
	var titles []string
	for _, p := range discord.posts {
		embeds, _ := p["embeds"].([]any)
		if len(embeds) != 1 {
			t.Fatalf("embeds per post = %d; want 1", len(embeds))
		}
		titles = append(titles, embeds[0].(map[string]any)["title"].(string))
	}
	if fmt.Sprint(titles) != "[Alpha Beta Gamma]" {
		t.Errorf("posted in the order %v", titles)
	}
}

func TestPublishUsesPerClubCommentsLabel(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	discord := useBookClubWebhook(t, e, "yuri", http.StatusNoContent, nil)

	listID := createReadingListIn(t, e, "yuri", "Yuri Faves")
	addItem(t, e, "yuri", listID, "Bloom Into You", map[string]any{"comments": "Drani loved it."})
	decodeInto(t, e.postJSON(t, "/api/v1/book-clubs/yuri/reading-lists/"+itoa(listID)+"/publish", nil), http.StatusOK, nil)

	embeds, _ := discord.posts[0]["embeds"].([]any)
	fields, _ := embeds[0].(map[string]any)["fields"].([]any)
	found := false
	for _, f := range fields {
		switch f.(map[string]any)["name"] {
		case "Drani's Comments":
			found = true
		case "Yao's Comments":
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
	var stub recordingDiscord
	stub.use(t, e, http.StatusNoContent, nil)
	listID := createReadingList(t, e, "No Hook")
	addItem(t, e, "yaoi", listID, "X", nil)

	_, code, detail := problem(t, e.postJSON(t, "/api/v1/book-clubs/yaoi/reading-lists/"+itoa(listID)+"/publish", nil))
	if code != http.StatusInternalServerError || !strings.Contains(detail, "No Discord webhook is set") {
		t.Fatalf("publish w/o webhook = %d %q; want 500 saying no webhook is set", code, detail)
	}
	if stub.count() != 0 {
		t.Error("Discord was called with no webhook set")
	}
}

func TestPublishEmptyListIsAConflict(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	useBookClubWebhook(t, e, "yaoi", http.StatusNoContent, nil)
	listID := createReadingList(t, e, "Empty")
	if got := status(e.postJSON(t, "/api/v1/book-clubs/yaoi/reading-lists/"+itoa(listID)+"/publish", nil)); got != http.StatusConflict {
		t.Errorf("publishing an empty list = %d; want 409", got)
	}
}

// Discord failing a post is answered with the status that says what to do, and a
// failure part-way says how far the run got.
func TestPublishDiscordFailures(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		header     http.Header
		want       int
		wantRetry  string
		wantDetail string
	}{
		{"refused", http.StatusNotFound, nil, http.StatusInternalServerError, "", "Posted 0 of 2, then: Discord refused the post"},
		{"rate limited", http.StatusTooManyRequests, http.Header{"Retry-After": {"4"}}, http.StatusTooManyRequests, "4", "4 seconds"},
		{"down", http.StatusBadGateway, nil, http.StatusServiceUnavailable, "", "can't be reached"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newTestEnv(t)
			e.loginAdmin(t)
			useBookClubWebhook(t, e, "yaoi", c.status, c.header)
			listID := createReadingList(t, e, "L")
			addItem(t, e, "yaoi", listID, "A", nil)
			addItem(t, e, "yaoi", listID, "B", nil)

			resp := e.postJSON(t, "/api/v1/book-clubs/yaoi/reading-lists/"+itoa(listID)+"/publish", nil)
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != c.want || resp.Header.Get("Retry-After") != c.wantRetry || !strings.Contains(string(body), c.wantDetail) {
				t.Errorf("%d Retry-After %q %s; want %d, %q, %q", resp.StatusCode, resp.Header.Get("Retry-After"), body, c.want, c.wantRetry, c.wantDetail)
			}
		})
	}
}

// Deleting a list or an item removes the uploaded covers no other item uses, and
// leaves a shared one and an AniList one alone.
func TestDeleteRemovesUnusedCovers(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	dir := filepath.Join(e.srv.DataDirForTest(), "images", "bookclub")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"shared.jpg", "solo.jpg", "list.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cover := func(name string) string { return e.ts.URL + "/images/bookclub/" + name }
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}

	first := createReadingList(t, e, "First")
	second := createReadingList(t, e, "Second")
	a := addItem(t, e, "yaoi", first, "A", map[string]any{"coverImage": cover("shared.jpg")})
	addItem(t, e, "yaoi", second, "B", map[string]any{"coverImage": cover("shared.jpg")})
	solo := addItem(t, e, "yaoi", first, "Solo", map[string]any{"coverImage": cover("solo.jpg")})
	addItem(t, e, "yaoi", second, "Remote", map[string]any{"coverImage": "https://s4.anilist.co/cover.jpg"})
	addItem(t, e, "yaoi", first, "InList", map[string]any{"coverImage": cover("list.jpg")})

	path := func(list int64, item map[string]any) string {
		return "/api/v1/book-clubs/yaoi/reading-lists/" + itoa(list) + "/items/" + itoa(int64(item["id"].(float64)))
	}
	if got := status(e.del(t, path(first, a))); got != http.StatusNoContent {
		t.Fatalf("delete shared-cover item = %d", got)
	}
	if !exists("shared.jpg") {
		t.Error("a cover another item still uses was deleted")
	}
	if got := status(e.del(t, path(first, solo))); got != http.StatusNoContent {
		t.Fatalf("delete solo-cover item = %d", got)
	}
	if exists("solo.jpg") {
		t.Error("a cover no item uses any more was kept")
	}

	// Deleting the first list takes its remaining item's cover with it; the second
	// list's items keep the shared one.
	if got := status(e.del(t, "/api/v1/book-clubs/yaoi/reading-lists/"+itoa(first))); got != http.StatusNoContent {
		t.Fatalf("delete list = %d", got)
	}
	if exists("list.jpg") {
		t.Error("a deleted list's cover was kept")
	}
	if !exists("shared.jpg") {
		t.Error("deleting a list removed a cover another list's item uses")
	}
}

// -- Title lookup -------------------------------------------------------------

const aniListReply = `{"data":{"Page":{"media":[{
	"id":111621,
	"title":{"romaji":"Yahwacheop","english":"Painter of the Night","native":"야화첩"},
	"description":"Na-kyum is a talented artist.<br><br>(Source: Seven Seas)",
	"coverImage":{"extraLarge":"https://s4.anilist.co/cover.jpg","large":"https://s4.anilist.co/med.jpg"},
	"format":"MANGA","countryOfOrigin":"KR","genres":["Drama","Romance"],
	"chapters":131,"siteUrl":"https://anilist.co/manga/111621"}]}}}`

// useAniList points the server's AniList client at a stub that answers with answer,
// keeping the saved endpoint a valid, allowlisted address.
func useAniList(t *testing.T, e *testEnv, answer http.HandlerFunc) {
	t.Helper()
	stub := httptest.NewServer(answer)
	t.Cleanup(stub.Close)
	if err := e.srv.RouteAnilistForTest(stub.URL); err != nil {
		t.Fatal(err)
	}
}

func TestLookUpTitleMapsAniListFields(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	var mu sync.Mutex
	var gotMethod, gotContentType string
	useAniList(t, e, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotMethod, gotContentType = r.Method, r.Header.Get("Content-Type")
		mu.Unlock()
		_, _ = io.WriteString(w, aniListReply)
	})

	var found struct{ Results []map[string]any }
	decodeInto(t, e.get(t, "/api/v1/book-clubs/yaoi/lookup?q=painter"), http.StatusOK, &found)
	mu.Lock()
	method, contentType := gotMethod, gotContentType
	mu.Unlock()
	if method != http.MethodPost || contentType != "application/json" {
		t.Errorf("upstream request = %s %q; want a JSON POST", method, contentType)
	}
	if len(found.Results) != 1 {
		t.Fatalf("results = %d; want 1", len(found.Results))
	}
	r := found.Results[0]
	if r["title"] != "Painter of the Night" || r["coverImage"] != "https://s4.anilist.co/cover.jpg" ||
		r["format"] != "Manhwa" || r["genres"] != "Drama, Romance" || r["chapters"] != "131" {
		t.Errorf("result = %v", r)
	}
	if s, _ := r["summary"].(string); strings.Contains(s, "<br>") || !strings.Contains(s, "Na-kyum is a talented artist.") {
		t.Errorf("summary not stripped cleanly: %q", s)
	}
	sources, _ := r["sources"].([]any)
	if len(sources) != 1 || sources[0].(map[string]any)["title"] != "AniList" || sources[0].(map[string]any)["url"] != "https://anilist.co/manga/111621" {
		t.Errorf("sources = %v; want the AniList link", sources)
	}

	// By id: the same mapping.
	decodeInto(t, e.get(t, "/api/v1/book-clubs/yaoi/lookup?id=111621"), http.StatusOK, &found)
}

func TestLookUpTitleNeedsOneOfQueryAndID(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	for _, path := range []string{"/lookup", "/lookup?q=", "/lookup?q=x&id=5"} {
		if at := invalid(t, e.get(t, "/api/v1/book-clubs/yaoi"+path)); !slices.Contains(at, "query.q") {
			t.Errorf("GET %s: errors at %v; want query.q", path, at)
		}
	}
	if got := status(e.get(t, "/api/v1/book-clubs/yaoi/lookup?id=abc")); got != http.StatusUnprocessableEntity {
		t.Errorf("a non-numeric id = %d; want 422", got)
	}
}

// AniList failing is answered with the status that says what to do - never a 424 or
// 502 - and says why in words.
func TestLookUpTitleFailures(t *testing.T) {
	cases := []struct {
		name       string
		answer     http.HandlerFunc
		want       int
		wantRetry  string
		wantDetail string
	}{
		{"disabled", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"errors":[{"message":"AniList API temporarily disabled"}]}`)
		}, http.StatusInternalServerError, "", "AniList refused the lookup: AniList API temporarily disabled"},
		{"rate limited", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
		}, http.StatusTooManyRequests, "30", "30 seconds"},
		{"down", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) },
			http.StatusServiceUnavailable, "", "can't be reached"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newTestEnv(t)
			e.loginAdmin(t)
			useAniList(t, e, c.answer)
			resp := e.get(t, "/api/v1/book-clubs/yaoi/lookup?q=x")
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != c.want || resp.Header.Get("Retry-After") != c.wantRetry || !strings.Contains(string(body), c.wantDetail) {
				t.Errorf("%d Retry-After %q %s; want %d, %q, %q", resp.StatusCode, resp.Header.Get("Retry-After"), body, c.want, c.wantRetry, c.wantDetail)
			}
		})
	}
}

// The saved endpoint is held to the AniList allowlist when it is used, not only when
// it is saved: one written another way falls back to the real AniList, never the
// internal address.
func TestLookUpTitleIgnoresAnEndpointOffTheAllowlist(t *testing.T) {
	e := newTestEnv(t)
	e.loginAdmin(t)
	var hit sync.Mutex
	var urls []string
	useAniList(t, e, func(w http.ResponseWriter, r *http.Request) {
		hit.Lock()
		urls = append(urls, r.URL.Path)
		hit.Unlock()
		_, _ = io.WriteString(w, aniListReply)
	})
	if err := e.store.SetSetting(t.Context(), "anilist_api_url", "https://169.254.169.254/latest"); err != nil {
		t.Fatal(err)
	}
	decodeInto(t, e.get(t, "/api/v1/book-clubs/yaoi/lookup?q=painter"), http.StatusOK, nil)
	// The stub rewrites the host, so the path tells which endpoint the request was
	// for: the default has none, the metadata address had /latest.
	hit.Lock()
	defer hit.Unlock()
	if len(urls) != 1 || urls[0] != "/" {
		t.Fatalf("the stub saw requests for %v; want one for the default endpoint", urls)
	}
}

// -- Cover uploads ------------------------------------------------------------

func TestBookClubCoverUpload(t *testing.T) {
	dataDir := t.TempDir()
	env := newTestEnvWithDataDir(t, dataDir)
	yaoi := makeActiveUser(t, env, "yaoistaff", "correct horse battery", []string{"bookclub-yaoi"})
	const path = "/api/v1/book-clubs/yaoi/covers"
	// A browser labels a picture by its type.
	cover := func(name string, data []byte) []filePart {
		kind := "image/png"
		if strings.HasSuffix(name, ".jpg") {
			kind = "image/jpeg"
		}
		return []filePart{{field: "image", name: name, data: data, kind: kind}}
	}

	// The club's own page is the guard: nobody 401, another club's page 403, an unknown club 404.
	env.signOut(t)
	resp := env.upload(t, path, nil, cover("a.png", realPNG(t, 2, 2))...)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("signed out = %d; want 401", resp.StatusCode)
	}
	yuri := makeActiveUser(t, env, "yuristaff", "correct horse battery", []string{"bookclub-yuri"})
	resp = uploadAs(t, yuri, env, path, nil, cover("a.png", realPNG(t, 2, 2))...)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("another club's page = %d; want 403", resp.StatusCode)
	}
	resp = uploadAs(t, yaoi, env, "/api/v1/book-clubs/nope/covers", nil, cover("a.png", realPNG(t, 2, 2))...)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("an unknown club = %d; want 404", resp.StatusCode)
	}

	// A picture is saved again without its metadata, under its own name, and answered
	// as an absolute, path-escaped address.
	var out struct {
		URL string `json:"url"`
	}
	decodeInto(t, uploadAs(t, yaoi, env, path, nil, cover("my cover.jpg", exifJPEG(t, 8, 4, 6))...), http.StatusCreated, &out)
	if !strings.HasPrefix(out.URL, "https://") || !strings.HasSuffix(out.URL, "/images/bookclub/my%20cover.jpg") {
		t.Errorf("url = %q; want an absolute, escaped address", out.URL)
	}
	stored, err := os.ReadFile(filepath.Join(dataDir, "images", "bookclub", "my cover.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte("Exif")) {
		t.Error("the stored cover still has its EXIF")
	}
	// The same name replaces it.
	decodeInto(t, uploadAs(t, yaoi, env, path, nil, cover("my cover.jpg", exifJPEG(t, 6, 6, 1))...), http.StatusCreated, nil)

	for name, parts := range map[string][]filePart{
		"a page named like a picture": cover("fake.png", []byte("<html><script>alert(1)</script></html>")),
		"a text file":                 cover("notes.txt", []byte("hello")),
		"an svg":                      cover("art.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)),
		"no file":                     nil,
	} {
		resp := uploadAs(t, yaoi, env, path, nil, parts...)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s = %d; want 422", name, resp.StatusCode)
		}
	}
	resp = uploadAs(t, yaoi, env, path, nil, cover("big.png", bytes.Repeat([]byte{0}, 7<<20))...)
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("a 7 MB cover = %d; want 413", resp.StatusCode)
	}
	if entries, _ := os.ReadDir(filepath.Join(dataDir, "images", "bookclub")); len(entries) != 1 {
		t.Errorf("the covers folder holds %d files; want only the one saved", len(entries))
	}
}
