package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"app-suite/internal/discord"
	"app-suite/internal/model"
)

// defaultAniListURL is the fallback AniList GraphQL endpoint (used when the
// anilist_api_url setting is unset). Configurable on the admin settings page.
// AniList needs no API key for public read queries.
const defaultAniListURL = "https://graphql.anilist.co"

// anilistUserAgent identifies this app to AniList (and its Cloudflare front).
// Sending a descriptive User-Agent instead of Go's default is good practice for
// public APIs and helps avoid being caught by generic bot filtering.
const anilistUserAgent = "SenpanAppSuite (+https://apps.senpan.cafe)"

// defaultClubSlug is the book club used when a request omits one. The schema
// supports more clubs via club_slug without changes.
const defaultClubSlug = "yaoi"

// bookClub identifies a book club for per-club configuration (notably its own
// Discord destination, "bookclub:<slug>"). Add an entry here to introduce a new club.
type bookClub struct {
	Slug string
	Name string
	// CommentsLabel is the label used for the per-item curator comments field in
	// the published Discord embed (e.g. "Yao's Comments", "Drani's Comments").
	CommentsLabel string
}

// bookClubs is the registry of known book clubs.
var bookClubs = []bookClub{
	{Slug: "yaoi", Name: "Yaoi Book Club", CommentsLabel: "Yao's Comments"},
	{Slug: "yuri", Name: "Yuri Book Club", CommentsLabel: "Drani's Comments"},
}

// commentsLabelForClub returns a club's curator-comments field label, falling
// back to a generic "Comments" for unknown slugs.
func commentsLabelForClub(slug string) string {
	for _, c := range bookClubs {
		if c.Slug == slug {
			return c.CommentsLabel
		}
	}
	return "Comments"
}

// isAllowedAniListURL reports whether raw is an acceptable AniList GraphQL
// endpoint: https on a host under anilist.co. The anilist_api_url setting drives
// the server's outbound lookup POST, so it is validated against this allowlist
// to stop an admin-entered value from
// pointing those requests at an internal service or cloud metadata endpoint.
func isAllowedAniListURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "anilist.co" || strings.HasSuffix(host, ".anilist.co")
}

// anilistHTTPClient is the client for AniList GraphQL lookups. The initial
// endpoint is admin-configured and validated against isAllowedAniListURL at
// settings-save time, but a redirect could still bounce the request to an
// internal host (SSRF). CheckRedirect re-validates EVERY hop against the same
// AniList allowlist, so a crafted/compromised endpoint can't redirect the
// server's outbound request to a private service or cloud metadata endpoint.
var anilistHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("stopped after %d redirects", len(via))
		}
		if !isAllowedAniListURL(req.URL.String()) {
			return fmt.Errorf("redirect to disallowed host %q blocked", req.URL.Host)
		}
		return nil
	},
}

// -- Reading lists (admin) ---------------------------------------------------

// bookClubFromPath reads the {club} path segment, defaults it, and enforces the
// caller holds that club's page permission. Writes the 401/403 itself and
// returns ok=false so the handler can return immediately. Because
// requirePermission already implies auth (an unauthenticated caller gets a 401),
// this single guard replaces the old requireAuth-then-load-then-check dance.
func (s *Server) bookClubFromPath(w http.ResponseWriter, r *http.Request) (string, bool) {
	club := strings.TrimSpace(r.PathValue("club"))
	if club == "" {
		club = defaultClubSlug
	}
	if !s.requirePermission(w, r, bookClubPerm(club)) {
		return "", false
	}
	return club, true
}

// handleReadingListsList returns all reading lists for a book club (no items).
//
//	Endpoint:  GET /api/book-clubs/{club}/reading-lists
//	Auth:      admin, or the book club's page permission
//	Response:  {"reading_lists": [...]}
func (s *Server) handleReadingListsList(w http.ResponseWriter, r *http.Request) {
	club, ok := s.bookClubFromPath(w, r)
	if !ok {
		return
	}
	lists, err := s.store.ListReadingLists(club)
	if err != nil {
		writeInternalError(w, "list reading lists", err)
		return
	}
	writeJSON(w, http.StatusOK, model.ReadingListsResponse{ReadingLists: lists})
}

// handleReadingListDetail returns a single reading list with its items.
//
//	Endpoint:  GET /api/book-clubs/{club}/reading-lists/{id}
//	Auth:      admin, or the book club's page permission
//	Response:  {"reading_list": ReadingList}
func (s *Server) handleReadingListDetail(w http.ResponseWriter, r *http.Request) {
	club, ok := s.bookClubFromPath(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid reading list ID")
		return
	}
	list, err := s.store.GetReadingList(id)
	if err != nil {
		writeInternalError(w, "get reading list", err)
		return
	}
	// Security guard: the list must belong to the club in the path, so a caller
	// with one club's permission can't read another club's list by guessing an id.
	if list == nil || list.ClubSlug != club {
		writeError(w, http.StatusNotFound, "Reading list not found")
		return
	}
	writeJSON(w, http.StatusOK, model.ReadingListDetailResponse{ReadingList: *list})
}

// readingListCreateRequest is the JSON body for
// POST /api/book-clubs/{club}/reading-lists. The owning club comes from the path.
type readingListCreateRequest struct {
	Title string `json:"title"`
}

// readingListUpdateRequest is the JSON body for
// PUT /api/book-clubs/{club}/reading-lists/{id}.
type readingListUpdateRequest struct {
	Title string `json:"title"`
}

// handleReadingListCreate creates a reading list. The owning club comes from the
// path; the caller must hold that club's page permission.
//
//	Endpoint:  POST /api/book-clubs/{club}/reading-lists
//	Auth:      admin, or the book club's page permission
//	Response:  201 {"reading_list": ReadingList}
func (s *Server) handleReadingListCreate(w http.ResponseWriter, r *http.Request) {
	club, ok := s.bookClubFromPath(w, r)
	if !ok {
		return
	}
	req, err := readJSON[readingListCreateRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "Title is required")
		return
	}
	id, err := s.store.CreateReadingList(club, title)
	if err != nil {
		writeInternalError(w, "create reading list", err)
		return
	}
	list, err := s.store.GetReadingList(id)
	if err != nil {
		writeInternalError(w, "load created reading list", err)
		return
	}
	writeJSON(w, http.StatusCreated, model.ReadingListDetailResponse{ReadingList: *list})
}

// handleReadingListUpdate renames a reading list. The club comes from the path;
// the loaded record must belong to it.
//
//	Endpoint:  PUT /api/book-clubs/{club}/reading-lists/{id}
//	Auth:      admin, or the book club's page permission
//	Response:  200 {"ok": true}
func (s *Server) handleReadingListUpdate(w http.ResponseWriter, r *http.Request) {
	club, ok := s.bookClubFromPath(w, r)
	if !ok {
		return
	}
	id, ok := pathInt64(w, r, "id", "reading list")
	if !ok {
		return
	}
	existing, err := s.store.GetReadingList(id)
	if err != nil {
		writeInternalError(w, "get reading list", err)
		return
	}
	// Security guard: the list must belong to the club in the path.
	if existing == nil || existing.ClubSlug != club {
		writeError(w, http.StatusNotFound, "Reading list not found")
		return
	}
	req, err := readJSON[readingListUpdateRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "Title is required")
		return
	}
	if err := s.store.UpdateReadingListTitle(id, title); err != nil {
		writeInternalError(w, "update reading list", err)
		return
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}

// handleReadingListDelete deletes a reading list. The club comes from the path;
// the loaded record must belong to it.
//
//	Endpoint:  DELETE /api/book-clubs/{club}/reading-lists/{id}
//	Auth:      admin, or the book club's page permission
//	Response:  204 No Content
func (s *Server) handleReadingListDelete(w http.ResponseWriter, r *http.Request) {
	club, ok := s.bookClubFromPath(w, r)
	if !ok {
		return
	}
	id, ok := pathInt64(w, r, "id", "reading list")
	if !ok {
		return
	}
	existing, err := s.store.GetReadingList(id)
	if err != nil {
		writeInternalError(w, "get reading list", err)
		return
	}
	// Security guard: the list must belong to the club in the path.
	if existing == nil || existing.ClubSlug != club {
		writeError(w, http.StatusNotFound, "Reading list not found")
		return
	}
	// Capture cover URLs before deleting the list (items cascade-delete in the
	// DB, but their files would otherwise be orphaned), then clean up each file
	// after the rows are gone - skipping any still referenced by another list.
	var covers []string
	for _, it := range existing.Items {
		covers = append(covers, it.CoverImage)
	}
	if _, err := s.store.DeleteReadingList(id); err != nil {
		writeInternalError(w, "delete reading list", err)
		return
	}
	for _, c := range covers {
		s.removeBookclubCoverIfUnused(c)
	}
	w.WriteHeader(http.StatusNoContent)
}

// readingListItemWriteRequest is the JSON body for creating (POST /api/
// book-clubs/{club}/reading-lists/{id}/items) or replacing (PUT /api/
// book-clubs/{club}/reading-lists/{id}/items/{itemId}) an item. The ids come
// from the path.
type readingListItemWriteRequest struct {
	Item model.ReadingListItem `json:"item"`
}

// resolveReadingListParent reads the {club} from the path, enforces its page
// permission, and loads the parent list for an /items sub-resource, writing the
// 401/403/404 itself. The list must belong to the club in the path (the same
// security guard as the list handlers). Returns (listID, ok).
func (s *Server) resolveReadingListParent(w http.ResponseWriter, r *http.Request) (int64, bool) {
	club, ok := s.bookClubFromPath(w, r)
	if !ok {
		return 0, false
	}
	listID, ok := pathInt64(w, r, "id", "reading list")
	if !ok {
		return 0, false
	}
	parent, err := s.store.GetReadingList(listID)
	if err != nil {
		writeInternalError(w, "get reading list", err)
		return 0, false
	}
	// Security guard: the parent list must belong to the club in the path.
	if parent == nil || parent.ClubSlug != club {
		writeError(w, http.StatusNotFound, "Reading list not found")
		return 0, false
	}
	return listID, true
}

// handleReadingListItemCreate creates an item within a list.
//
//	Endpoint:  POST /api/book-clubs/{club}/reading-lists/{id}/items
//	Auth:      admin, or the book club's page permission
//	Response:  201 {"item": ReadingListItem}
func (s *Server) handleReadingListItemCreate(w http.ResponseWriter, r *http.Request) {
	listID, ok := s.resolveReadingListParent(w, r)
	if !ok {
		return
	}
	req, err := readJSON[readingListItemWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	title := strings.TrimSpace(req.Item.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "Item title is required")
		return
	}
	it := req.Item
	it.ListID = listID
	it.Title = title
	it.Sources = sanitizeSources(it.Sources)
	id, err := s.store.CreateReadingListItem(&it)
	if err != nil {
		writeInternalError(w, "create reading list item", err)
		return
	}
	it.ID = id
	writeJSON(w, http.StatusCreated, model.ReadingListItemResponse{Item: it})
}

// handleReadingListItemUpdate replaces an item within a list.
//
//	Endpoint:  PUT /api/book-clubs/{club}/reading-lists/{id}/items/{itemId}
//	Auth:      admin, or the book club's page permission
//	Response:  200 {"item": ReadingListItem}
func (s *Server) handleReadingListItemUpdate(w http.ResponseWriter, r *http.Request) {
	listID, ok := s.resolveReadingListParent(w, r)
	if !ok {
		return
	}
	itemID, ok := pathInt64(w, r, "itemId", "item")
	if !ok {
		return
	}
	req, err := readJSON[readingListItemWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	title := strings.TrimSpace(req.Item.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "Item title is required")
		return
	}
	it := req.Item
	it.ID = itemID
	it.ListID = listID
	it.Title = title
	it.Sources = sanitizeSources(it.Sources)
	updated, err := s.store.UpdateReadingListItem(&it)
	if err != nil {
		writeInternalError(w, "update reading list item", err)
		return
	}
	// No match means the item is not on this list - a stale editor, or an id
	// pointed at another club's list. 404 rather than 403: whether that id exists
	// elsewhere is not this caller's business.
	if !updated {
		writeError(w, http.StatusNotFound, "Reading list item not found")
		return
	}
	writeJSON(w, http.StatusOK, model.ReadingListItemResponse{Item: it})
}

// handleReadingListItemDelete removes an item from a list, cleaning up its cover
// image afterwards when no other item still references it.
//
//	Endpoint:  DELETE /api/book-clubs/{club}/reading-lists/{id}/items/{itemId}
//	Auth:      admin, or the book club's page permission
//	Response:  204 No Content
func (s *Server) handleReadingListItemDelete(w http.ResponseWriter, r *http.Request) {
	listID, ok := s.resolveReadingListParent(w, r)
	if !ok {
		return
	}
	itemID, ok := pathInt64(w, r, "itemId", "item")
	if !ok {
		return
	}
	// Capture the cover URL before deleting so the file can be cleaned up after
	// the row is gone - but only if no other item still references it.
	var cover string
	if list, err := s.store.GetReadingList(listID); err == nil && list != nil {
		for _, it := range list.Items {
			if it.ID == itemID {
				cover = it.CoverImage
				break
			}
		}
	}
	deleted, err := s.store.DeleteReadingListItem(listID, itemID)
	if err != nil {
		writeInternalError(w, "delete reading list item", err)
		return
	}
	// Not on this list: a stale editor, or an id belonging to another club's list.
	// Report it missing rather than 204ing on a delete that did nothing.
	if !deleted {
		writeError(w, http.StatusNotFound, "Reading list item not found")
		return
	}
	s.removeBookclubCoverIfUnused(cover)
	w.WriteHeader(http.StatusNoContent)
}

// sanitizeSources trims and drops empty source rows so blank repeater entries
// from the form aren't persisted. A source needs at least a URL.
func sanitizeSources(sources []model.ReadingListSource) []model.ReadingListSource {
	out := make([]model.ReadingListSource, 0, len(sources))
	for _, src := range sources {
		title := strings.TrimSpace(src.Title)
		u := strings.TrimSpace(src.URL)
		if u == "" {
			continue
		}
		out = append(out, model.ReadingListSource{Title: title, URL: u})
	}
	return out
}

// -- Cover image upload ------------------------------------------------------

// handleBookclubUpload handles multipart cover-image uploads for reading list
// items, storing them under <webRoot>/images/bookclub and returning the full
// URL (per the requirement that covers are stored as full URLs).
//
//	Endpoint:  POST /api/bookclub/upload
//	Auth:      admin, or any book-club page permission
//	Request:   multipart form with "image" field
//	Response:  {"url": "https://host/images/bookclub/<original-filename>"}
func (s *Server) handleBookclubUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireAnyBookClub(w, r) {
		return
	}
	s.saveSingleImageUpload(w, r, bookclubCoverRelDir)
}

// siteBaseURL reconstructs this site's scheme://host for building absolute URLs
// to uploaded assets. Honors X-Forwarded-Proto (the app runs behind a reverse
// proxy) and falls back to TLS detection.
func (s *Server) siteBaseURL(r *http.Request) string {
	scheme := "https"
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = proto
	} else if r.TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

// removeBookclubCoverIfUnused deletes an uploaded cover file, but only when no
// reading-list item still references it. Covers keep their uploaded filename (the
// app no longer rewrites upload names), so two items can share one file; call this
// AFTER the owning item/list rows are deleted so the reference count reflects only
// the remaining items. External (AniList) URLs are ignored by the inner removal.
func (s *Server) removeBookclubCoverIfUnused(coverURL string) {
	if coverURL == "" {
		return
	}
	if n, err := s.store.CountReadingListItemsByCover(coverURL); err != nil || n > 0 {
		return
	}
	s.removeUploadedBookclubImage(coverURL)
}

// removeUploadedBookclubImage deletes a previously uploaded cover image, but
// only when the URL points inside this site's images/bookclub directory. API
// covers (e.g. s4.anilist.co) and anything outside the upload area are left
// untouched.
func (s *Server) removeUploadedBookclubImage(coverURL string) {
	if coverURL == "" {
		return
	}
	// Reduce a full URL to its path so we can resolve it against webRoot.
	path := coverURL
	if u, err := url.Parse(coverURL); err == nil && u.Path != "" {
		path = u.Path
	}
	path = strings.TrimPrefix(path, "/")
	if !strings.HasPrefix(path, "images/bookclub/") {
		return
	}
	uploadDir, err := filepath.Abs(filepath.Join(s.webRoot, "images", "bookclub"))
	if err != nil {
		return
	}
	target, err := filepath.Abs(filepath.Join(s.webRoot, filepath.Clean(path)))
	if err != nil {
		return
	}
	if target == uploadDir || !strings.HasPrefix(target, uploadDir+string(os.PathSeparator)) {
		return
	}
	_ = os.Remove(target)
}

// -- AniList lookup ----------------------------------------------------------

// anilistMedia is the subset of the AniList Media object we map to a reading
// list item. AniList is a GraphQL API; these fields match the selection set in
// anilistMediaFields. Other response fields are ignored.
type anilistMedia struct {
	ID    int `json:"id"`
	Title struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
		Native  string `json:"native"`
	} `json:"title"`
	Description string `json:"description"`
	CoverImage  struct {
		ExtraLarge string `json:"extraLarge"`
		Large      string `json:"large"`
	} `json:"coverImage"`
	Format          string   `json:"format"`
	CountryOfOrigin string   `json:"countryOfOrigin"`
	Genres          []string `json:"genres"`
	Chapters        *int     `json:"chapters"`
	SiteURL         string   `json:"siteUrl"`
}

// anilistMediaFields is the shared GraphQL selection set for a Media node.
const anilistMediaFields = `id title { romaji english native } description(asHtml: false) ` +
	`coverImage { extraLarge large } format countryOfOrigin genres chapters siteUrl`

// handleBookclubLookup proxies the AniList GraphQL API and maps results to
// reading-list item suggestions (so the browser avoids CORS and field mapping
// lives in one place). Pass ?q= to search (manga), or ?id= for a single title
// by its AniList id.
//
//	Endpoint:  GET /api/bookclub/lookup?q=<query>  |  ?id=<anilist id>
//	Auth:      admin, or any book-club page permission
//	Response:  {"results": [ReadingListItem-shaped, ...]}
//	Errors:    424 Failed Dependency when AniList is unavailable - a 4xx, not a
//	           5xx, on purpose: Cloudflare replaces origin 5xx bodies with its own
//	           error page, so a 5xx would hide the real reason from the client.
func (s *Server) handleBookclubLookup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAnyBookClub(w, r) {
		return
	}
	endpoint := s.anilistURL()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	idStr := strings.TrimSpace(r.URL.Query().Get("id"))

	var media []anilistMedia
	var err error
	switch {
	case idStr != "":
		id, convErr := strconv.Atoi(idStr)
		if convErr != nil {
			writeError(w, http.StatusBadRequest, "Invalid AniList id")
			return
		}
		var one *anilistMedia
		one, err = fetchAniListByID(endpoint, id)
		if one != nil {
			media = []anilistMedia{*one}
		}
	case q != "":
		media, err = fetchAniListSearch(endpoint, q)
	default:
		writeError(w, http.StatusBadRequest, "Provide a search query (q) or an AniList id")
		return
	}
	if err != nil {
		// Return 424 (Failed Dependency), not 502. This is genuinely a failed
		// upstream dependency, but Cloudflare fronts the site and replaces origin
		// *5xx* responses with its own branded error page - discarding our JSON
		// body, so the SPA only ever sees a bodyless 502. Cloudflare passes 4xx
		// bodies through untouched, so a 4xx is what actually delivers the real
		// reason (e.g. "AniList API temporarily disabled... (status 403)") to the
		// client. See handleBookclubLookup's doc comment.
		writeError(w, http.StatusFailedDependency, "AniList request failed: "+err.Error())
		return
	}

	results := make([]model.ReadingListItem, 0, len(media))
	for _, m := range media {
		results = append(results, anilistToItem(m))
	}
	writeJSON(w, http.StatusOK, model.BookclubLookupResponse{Results: results})
}

// anilistURL returns the configured AniList GraphQL endpoint (trimmed, no
// trailing slash), falling back to the default.
func (s *Server) anilistURL() string {
	base, _ := s.store.GetSetting("anilist_api_url")
	base = strings.TrimSpace(base)
	// Defense in depth: the settings handler validates anilist_api_url against
	// isAllowedAniListURL at save time, but a value written another way (a
	// migration, a direct DB edit) must not send our outbound lookup POST to an
	// internal service or cloud-metadata host. Re-check the stored value at read
	// time and fall back to the default endpoint when it isn't an allowed AniList URL.
	if base == "" || !isAllowedAniListURL(base) {
		base = defaultAniListURL
	}
	return strings.TrimRight(base, "/")
}

// fetchAniListSearch runs a paged manga search and returns the media list.
func fetchAniListSearch(endpoint, search string) ([]anilistMedia, error) {
	query := `query ($search: String) { Page(page: 1, perPage: 10) { media(search: $search, type: MANGA, sort: SEARCH_MATCH) { ` + anilistMediaFields + ` } } }`
	body, err := anilistPost(endpoint, query, map[string]any{"search": search})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			Page struct {
				Media []anilistMedia `json:"media"`
			} `json:"Page"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode response")
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("%s", resp.Errors[0].Message)
	}
	return resp.Data.Page.Media, nil
}

// fetchAniListByID fetches a single manga title by AniList id.
func fetchAniListByID(endpoint string, id int) (*anilistMedia, error) {
	query := `query ($id: Int) { Media(id: $id, type: MANGA) { ` + anilistMediaFields + ` } }`
	body, err := anilistPost(endpoint, query, map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			Media *anilistMedia `json:"Media"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode response")
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("%s", resp.Errors[0].Message)
	}
	return resp.Data.Media, nil
}

// anilistPost executes a GraphQL POST and returns the response body, capping
// the read size.
func anilistPost(endpoint, query string, variables map[string]any) ([]byte, error) {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return nil, fmt.Errorf("encode query")
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", anilistUserAgent)
	slog.Debug("anilist request", "endpoint", endpoint, "bytes", len(payload))
	resp, err := anilistHTTPClient.Do(req)
	if err != nil {
		// Don't swallow the transport failure - surface *why* AniList was
		// unreachable (timeout, connection reset, DNS/TLS) so it reaches the
		// operator instead of an opaque "request failed". http.Client wraps these
		// in *url.Error; a datacenter IP hitting AniList's Cloudflare front
		// commonly sees a timeout or reset rather than a clean HTTP status.
		if urlErr, ok := errors.AsType[*url.Error](err); ok {
			if urlErr.Timeout() {
				return nil, fmt.Errorf("did not respond within %s", anilistHTTPClient.Timeout)
			}
			return nil, fmt.Errorf("could not connect (%w)", urlErr.Err)
		}
		return nil, fmt.Errorf("request failed (%w)", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MB cap
	slog.Debug("anilist response", "status", resp.StatusCode, "bytes", len(body))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// AniList returns its GraphQL error shape even on non-2xx (e.g. a 403 when
		// the API is disabled for stability). Surface that message so the operator
		// sees the real reason ("AniList API temporarily disabled...") rather than a
		// bare status code.
		if msg := anilistErrorMessage(body); msg != "" {
			return nil, fmt.Errorf("%s (status %d)", msg, resp.StatusCode)
		}
		return nil, fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	return body, nil
}

// anilistErrorMessage extracts the first GraphQL error message from an AniList
// response body, returning "" when the body isn't the expected error shape.
func anilistErrorMessage(body []byte) string {
	var resp struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Errors) == 0 {
		return ""
	}
	return strings.TrimSpace(resp.Errors[0].Message)
}

// anilistToItem maps an AniList Media to a reading-list item suggestion: picks
// the best available title, strips HTML from the description, derives a friendly
// format from format + country of origin, and prefills the AniList page source.
func anilistToItem(m anilistMedia) model.ReadingListItem {
	cover := m.CoverImage.ExtraLarge
	if cover == "" {
		cover = m.CoverImage.Large
	}
	title := m.Title.English
	if title == "" {
		title = m.Title.Romaji
	}
	if title == "" {
		title = m.Title.Native
	}
	chapters := ""
	if m.Chapters != nil && *m.Chapters > 0 {
		chapters = strconv.Itoa(*m.Chapters)
	}
	sources := []model.ReadingListSource{}
	if m.SiteURL != "" {
		sources = append(sources, model.ReadingListSource{Title: "AniList", URL: m.SiteURL})
	}
	return model.ReadingListItem{
		CoverImage: cover,
		Title:      strings.TrimSpace(title),
		Summary:    stripHTML(m.Description),
		Format:     deriveFormat(m.Format, m.CountryOfOrigin),
		Genres:     strings.Join(m.Genres, ", "),
		Chapters:   chapters,
		Sources:    sources,
	}
}

// deriveFormat turns AniList's format enum + country of origin into a friendlier
// label (Manga / Manhwa / Manhua / Danmei / Novel). The admin can override it.
func deriveFormat(format, country string) string {
	switch format {
	case "NOVEL":
		if country == "CN" || country == "TW" {
			return "Danmei"
		}
		return "Novel"
	case "ONE_SHOT":
		return "One-shot"
	}
	switch country {
	case "KR":
		return "Manhwa"
	case "CN", "TW":
		return "Manhua"
	case "JP":
		return "Manga"
	}
	// Fall back to a title-cased version of the raw enum (e.g. "MANGA" -> "Manga").
	if format == "" {
		return ""
	}
	return strings.ToUpper(format[:1]) + strings.ToLower(format[1:])
}

// brTagRe matches AniList <br> line breaks; htmlTagRe matches any other tag.
var (
	brTagRe   = regexp.MustCompile(`(?i)<br\s*/?>`)
	htmlTagRe = regexp.MustCompile(`(?s)<[^>]*>`)
	blankRe   = regexp.MustCompile(`\n{3,}`)
)

// stripHTML removes HTML tags (AniList descriptions use <br>, <i>, <b>, ...),
// unescapes entities, and collapses runs of blank lines so the summary reads
// cleanly as plain text.
func stripHTML(s string) string {
	s = brTagRe.ReplaceAllString(s, "\n") // keep paragraph breaks
	s = htmlTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = blankRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// -- Discord publish ---------------------------------------------------------

// bookClubName is a club's display name ("Yaoi Book Club"), or its slug.
func bookClubName(slug string) string {
	for _, c := range bookClubs {
		if c.Slug == slug {
			return c.Name
		}
	}
	return slug
}

// handlePublishReadingList posts a reading list to the club's channel, one message per
// item (so long summaries fit Discord's 6000-character message budget), the ping on the
// first. All of a publish's messages share one batch, so the whole list can be replaced
// later:
//
//   - a club set to "replace" deletes its previous reading list once the new one is up;
//   - a publish that fails partway deletes what it had posted, so the channel never holds
//     half a list - and the previous list stays where it was.
//
// The writes run detached from the request, so a browser that gives up mid-publish
// doesn't leave half a list behind.
//
//	Endpoint:  POST /api/book-clubs/{club}/reading-lists/{id}/publish
//	Auth:      admin, or the book club's page permission
//	Response:  model.PublishResponse
func (s *Server) handlePublishReadingList(w http.ResponseWriter, r *http.Request) {
	club, ok := s.bookClubFromPath(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid reading list ID")
		return
	}

	list, err := s.store.GetReadingList(id)
	if err != nil {
		writeInternalError(w, "get reading list for publish", err)
		return
	}
	// Security guard: the list must belong to the club in the path.
	if list == nil || list.ClubSlug != club {
		writeError(w, http.StatusNotFound, "Reading list not found")
		return
	}
	if len(list.Items) == 0 {
		writeError(w, http.StatusBadRequest, "This reading list has no items to publish")
		return
	}
	dest, err := s.store.GetDiscordDestination(bookClubDestination(club))
	if err != nil {
		writeInternalError(w, "read book club destination", err)
		return
	}

	// One club publishes at a time: two overlapping publishes would each think the
	// other's messages were the previous list.
	unlock := s.discordItemLock(postKindReadingList+":"+club, 0)
	defer unlock()
	previous, err := s.store.LiveDiscordPostsInScope(postKindReadingList, club)
	if err != nil {
		writeInternalError(w, "read previous reading list posts", err)
		return
	}

	ctx, cancel := detachedDiscordContext(r, 2*time.Minute+time.Duration(len(list.Items))*5*time.Second)
	defer cancel()
	now := time.Now()
	batch := strconv.FormatInt(list.ID, 36) + "-" + strconv.FormatInt(now.UnixNano(), 36)
	commentsLabel := commentsLabelForClub(club)
	var posted []model.DiscordPost
	note := ""
	for i, it := range list.Items {
		mention := ""
		if i == 0 {
			mention = dest.Mention
		}
		created, err := s.discordCreate(ctx, discordOut{
			Kind: postKindReadingList, RefID: list.ID, Scope: club, Batch: batch, Seq: i,
			Name: it.Title, ChannelID: dest.ChannelID, Mention: mention,
			Message: discord.Message{Embeds: []discord.Embed{buildItemEmbed(it, commentsLabel)}},
			Patient: true,
		})
		if err != nil {
			rolledBack := s.rollBackBatch(posted, list.Title)
			slog.Error("publish reading list failed", "list_id", id, "item", it.Title,
				"posted", len(posted), "rolled_back", rolledBack, "error", err)
			if len(posted) == 0 {
				writeDiscordFailure(w, fmt.Sprintf("publish reading list %d", id), bookClubName(club), err)
				return
			}
			writeError(w, http.StatusBadGateway, fmt.Sprintf(
				"Publishing stopped at %q: %s The %d message(s) already posted were removed, so the channel is as it was.",
				it.Title, discordFailureText(err, bookClubName(club)), rolledBack))
			return
		}
		if created.Suppressed {
			writeJSON(w, http.StatusOK, model.PublishResponse{
				Note: "This server is in dry-run mode, so nothing was sent to Discord."})
			return
		}
		if created.Redirected != "" {
			note = "Posted to the test channel, without pings (" + created.Redirected + ")."
		}
		posted = append(posted, created.Post)
	}

	replaced := 0
	if dest.RepostMode == model.RepostReplace && len(previous) > 0 {
		var held int
		var delErr error
		old := previous
		for _, p := range posted {
			old = otherPosts(old, p.MessageID)
		}
		replaced, held, delErr = s.discordDeleteAll(ctx, old, list.Title)
		switch {
		case delErr != nil:
			slog.Error("remove previous reading list", "club", club, "error", delErr)
			note = joinNotes(note, "Some of the previous reading list couldn't be removed: "+discordErrorText(delErr))
		case held > 0:
			note = joinNotes(note, "The previous reading list is outside the test channel, so it was left up.")
		}
	}
	slog.Info("published reading list", "list_id", id, "list", list.Title, "club", club,
		"items", len(posted), "replaced", replaced, "by", s.actorName(r))
	writeJSON(w, http.StatusOK, model.PublishResponse{Published: len(posted), Replaced: replaced, Note: note})
}

// rollBackBatch deletes the messages a failed publish had posted, returning how many went.
func (s *Server) rollBackBatch(posted []model.DiscordPost, name string) int {
	if len(posted) == 0 {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	deleted, _, err := s.discordDeleteAll(ctx, posted, name)
	if err != nil {
		slog.Error("roll back a failed publish", "name", name, "error", err)
	}
	return deleted
}

// buildItemEmbed converts a reading list item into a Discord embed. The cover
// renders as the large, full-width embed image (not the small thumbnail), and
// the curator-comments field is labeled per club (commentsLabel).
func buildItemEmbed(it model.ReadingListItem, commentsLabel string) discord.Embed {
	b := discord.NewEmbed().
		Title(it.Title).
		Description(it.Summary).
		Image(it.CoverImage).
		Field("Format", it.Format, true).
		Field("Chapters", it.Chapters, true).
		Field("Genres", it.Genres, false).
		Field("Tropes", it.Tropes, false).
		Field(commentsLabel, it.Comments, false)

	if len(it.Sources) > 0 {
		links := make([]string, 0, len(it.Sources))
		for _, src := range it.Sources {
			if strings.TrimSpace(src.URL) == "" {
				continue
			}
			label := strings.TrimSpace(src.Title)
			if label == "" {
				label = src.URL
			}
			links = append(links, fmt.Sprintf("[%s](%s)", label, src.URL))
		}
		b.Field("Sources", strings.Join(links, "\n"), false)
	}
	return b.Build()
}
