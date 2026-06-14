package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"app-suite/internal/model"
)

// defaultAniListURL is the fallback AniList GraphQL endpoint (used when the
// anilist_api_url setting is unset). Configurable on the admin settings page.
// AniList needs no API key for public read queries.
const defaultAniListURL = "https://graphql.anilist.co"

// defaultClubSlug is the book club used when a request omits one. The schema
// supports more clubs via club_slug without changes.
const defaultClubSlug = "yaoi"

// bookClub identifies a book club for per-club configuration (notably its own
// Discord webhook). Add an entry here to introduce a new club; its webhook
// setting is then exposed/saved automatically (see init below).
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

// webhookSettingKey returns the per-club reading-list Discord webhook setting
// key for a slug (e.g. "discord_webhook_url_yaoi"). Must match clubWebhookKey
// on the frontend.
func webhookSettingKey(slug string) string {
	return "discord_webhook_url_" + slug
}

// eventsWebhookSettingKey returns the per-club events-channel Discord webhook
// setting key for a slug (e.g. "discord_events_webhook_url_yaoi"). Must match
// clubEventsWebhookKey on the frontend.
func eventsWebhookSettingKey(slug string) string {
	return "discord_events_webhook_url_" + slug
}

// init registers each club's Discord webhooks (reading-list + events channel)
// as secret app settings so the settings API exposes them to admins and accepts
// saves, without hard-coding a key per club in settings.go.
func init() {
	for _, c := range bookClubs {
		for _, key := range []string{webhookSettingKey(c.Slug), eventsWebhookSettingKey(c.Slug)} {
			settingsKeys = append(settingsKeys, key)
			secretSettings[key] = true
		}
	}
}

// bookclubHTTPClient is used for outbound calls (AniList lookups, Discord
// webhook posts) with a sane timeout so a slow upstream can't hang a request.
var bookclubHTTPClient = &http.Client{Timeout: 15 * time.Second}

// ── Reading lists (admin) ───────────────────────────────────────────────────

// handleReadingListsList returns all reading lists for a book club (no items).
//
//	Endpoint:  GET /api/reading-lists?club=yaoi
//	Auth:      admin
//	Response:  {"reading_lists": [...]}
func (s *Server) handleReadingListsList(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	club := strings.TrimSpace(r.URL.Query().Get("club"))
	if club == "" {
		club = defaultClubSlug
	}
	lists, err := s.store.ListReadingLists(club)
	if err != nil {
		writeInternalError(w, "list reading lists", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reading_lists": lists})
}

// handleReadingListDetail returns a single reading list with its items.
//
//	Endpoint:  GET /api/reading-lists/{id}
//	Auth:      admin
//	Response:  {"reading_list": ReadingList}
func (s *Server) handleReadingListDetail(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
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
	if list == nil {
		writeError(w, http.StatusNotFound, "Reading list not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reading_list": list})
}

// readingListRequest is the JSON body for POST /api/reading-lists.
type readingListRequest struct {
	Action   string `json:"action"`
	ID       int64  `json:"id"`
	ClubSlug string `json:"club_slug"`
	Title    string `json:"title"`
}

// handleReadingListsAction creates, renames, or deletes a reading list.
//
//	Endpoint:  POST /api/reading-lists
//	Auth:      admin
//	Request:   {"action": "create"|"update"|"delete", ...}
func (s *Server) handleReadingListsAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	req, err := readJSON[readingListRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Action {
	case "create":
		title := strings.TrimSpace(req.Title)
		if title == "" {
			writeError(w, http.StatusBadRequest, "Title is required")
			return
		}
		club := strings.TrimSpace(req.ClubSlug)
		if club == "" {
			club = defaultClubSlug
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
		writeJSON(w, http.StatusCreated, map[string]any{"reading_list": list})

	case "update":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Reading list id is required")
			return
		}
		title := strings.TrimSpace(req.Title)
		if title == "" {
			writeError(w, http.StatusBadRequest, "Title is required")
			return
		}
		if err := s.store.UpdateReadingListTitle(req.ID, title); err != nil {
			writeInternalError(w, "update reading list", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case "delete":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Reading list id is required")
			return
		}
		// Remove any uploaded cover images before deleting the list (items
		// cascade-delete in the DB, but their files would otherwise be orphaned).
		if list, err := s.store.GetReadingList(req.ID); err == nil && list != nil {
			for _, it := range list.Items {
				s.removeUploadedBookclubImage(it.CoverImage)
			}
		}
		deleted, err := s.store.DeleteReadingList(req.ID)
		if err != nil {
			writeInternalError(w, "delete reading list", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: create, update, delete")
	}
}

// readingListItemRequest is the JSON body for POST /api/reading-lists/{id}/items.
type readingListItemRequest struct {
	Action string                `json:"action"`
	ItemID int64                 `json:"item_id"`
	Item   model.ReadingListItem `json:"item"`
}

// handleReadingListItems creates, updates, or deletes an item within a list.
//
//	Endpoint:  POST /api/reading-lists/{id}/items
//	Auth:      admin
//	Request:   {"action": "create"|"update"|"delete", "item": {...}, "item_id": N}
func (s *Server) handleReadingListItems(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	listID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid reading list ID")
		return
	}
	req, err := readJSON[readingListItemRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Action {
	case "create":
		title := strings.TrimSpace(req.Item.Title)
		if title == "" {
			writeError(w, http.StatusBadRequest, "Item title is required")
			return
		}
		// Ensure the parent list exists so we don't create orphans.
		list, err := s.store.GetReadingList(listID)
		if err != nil {
			writeInternalError(w, "get list for item create", err)
			return
		}
		if list == nil {
			writeError(w, http.StatusNotFound, "Reading list not found")
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
		writeJSON(w, http.StatusCreated, map[string]any{"item": it})

	case "update":
		if req.ItemID <= 0 {
			writeError(w, http.StatusBadRequest, "Item id is required")
			return
		}
		title := strings.TrimSpace(req.Item.Title)
		if title == "" {
			writeError(w, http.StatusBadRequest, "Item title is required")
			return
		}
		it := req.Item
		it.ID = req.ItemID
		it.ListID = listID
		it.Title = title
		it.Sources = sanitizeSources(it.Sources)
		if err := s.store.UpdateReadingListItem(&it); err != nil {
			writeInternalError(w, "update reading list item", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": it})

	case "delete":
		if req.ItemID <= 0 {
			writeError(w, http.StatusBadRequest, "Item id is required")
			return
		}
		// Best-effort cleanup of the uploaded cover image, if any.
		if list, err := s.store.GetReadingList(listID); err == nil && list != nil {
			for _, it := range list.Items {
				if it.ID == req.ItemID {
					s.removeUploadedBookclubImage(it.CoverImage)
					break
				}
			}
		}
		deleted, err := s.store.DeleteReadingListItem(req.ItemID)
		if err != nil {
			writeInternalError(w, "delete reading list item", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: create, update, delete")
	}
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

// ── Cover image upload ──────────────────────────────────────────────────────

// handleBookclubUpload handles multipart cover-image uploads for reading list
// items, storing them under <webRoot>/images/bookclub and returning the full
// URL (per the requirement that covers are stored as full URLs).
//
//	Endpoint:  POST /api/bookclub/upload
//	Auth:      admin
//	Request:   multipart form with "image" field
//	Response:  {"url": "https://host/images/bookclub/cover_....ext"}
func (s *Server) handleBookclubUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	s.saveSingleImageUpload(w, r, bookclubCoverRelDir, "cover")
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

// removeUploadedBookclubImage deletes a previously uploaded cover image, but
// only when the URL points inside this site's images/bookclub directory. API
// covers (e.g. s4.anilist.co) and anything outside the upload area are left
// untouched. Mirrors removeUploadedImage (raffles).
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

// ── AniList lookup ──────────────────────────────────────────────────────────

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
//	Auth:      admin
//	Response:  {"results": [ReadingListItem-shaped, ...]}
func (s *Server) handleBookclubLookup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
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
		writeError(w, http.StatusBadGateway, "AniList request failed: "+err.Error())
		return
	}

	results := make([]model.ReadingListItem, 0, len(media))
	for _, m := range media {
		results = append(results, anilistToItem(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// anilistURL returns the configured AniList GraphQL endpoint (trimmed, no
// trailing slash), falling back to the default.
func (s *Server) anilistURL() string {
	base, _ := s.store.GetSetting("anilist_api_url")
	base = strings.TrimSpace(base)
	if base == "" {
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
	resp, err := bookclubHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MB cap
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	return body, nil
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
	// Fall back to a title-cased version of the raw enum (e.g. "MANGA" → "Manga").
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

// stripHTML removes HTML tags (AniList descriptions use <br>, <i>, <b>, …),
// unescapes entities, and collapses runs of blank lines so the summary reads
// cleanly as plain text.
func stripHTML(s string) string {
	s = brTagRe.ReplaceAllString(s, "\n") // keep paragraph breaks
	s = htmlTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = blankRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// ── Discord publish ─────────────────────────────────────────────────────────
//
// The embed schema types, builder, colour helper, and transport live in
// embeds.go (shared by every webhook-posting feature).

// handlePublishReadingList posts each item in a reading list as its own Discord
// embed via the configured webhook. Always posts every item (no published-state
// tracking).
//
//	Endpoint:  POST /api/reading-lists/{id}/publish
//	Auth:      admin
//	Response:  {"published": <count>}
func (s *Server) handlePublishReadingList(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
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
	if list == nil {
		writeError(w, http.StatusNotFound, "Reading list not found")
		return
	}
	if len(list.Items) == 0 {
		writeError(w, http.StatusBadRequest, "This reading list has no items to publish")
		return
	}

	// Each book club publishes to its own Discord channel (per-club webhook).
	webhook, _ := s.store.GetSetting(webhookSettingKey(list.ClubSlug))
	webhook = strings.TrimSpace(webhook)
	if webhook == "" {
		writeError(w, http.StatusBadRequest, "No Discord webhook configured for this book club. Set it under System → Settings.")
		return
	}

	commentsLabel := commentsLabelForClub(list.ClubSlug)
	published := 0
	for _, it := range list.Items {
		if err := postDiscordEmbed(webhook, buildItemEmbed(it, commentsLabel)); err != nil {
			writeError(w, http.StatusBadGateway,
				fmt.Sprintf("Published %d of %d before failing on %q: %v", published, len(list.Items), it.Title, err))
			return
		}
		published++
		// Be gentle with Discord's rate limit between posts.
		time.Sleep(350 * time.Millisecond)
	}

	writeJSON(w, http.StatusOK, map[string]any{"published": published})
}

// buildItemEmbed converts a reading list item into a Discord embed. The
// curator-comments field is labeled per club (commentsLabel).
func buildItemEmbed(it model.ReadingListItem, commentsLabel string) discordEmbed {
	b := newEmbed().
		title(it.Title).
		description(it.Summary).
		thumbnail(it.CoverImage).
		field("Format", it.Format, true).
		field("Chapters", it.Chapters, true).
		field("Genres", it.Genres, false).
		field("Tropes", it.Tropes, false).
		field(commentsLabel, it.Comments, false)

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
		b.field("Sources", strings.Join(links, "\n"), false)
	}
	return b.build()
}
