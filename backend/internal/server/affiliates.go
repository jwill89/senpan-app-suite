package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"app-suite/internal/model"
)

// ── Affiliates admin (list + CRUD + drag order + post to Discord) ────────────
//
// An affiliate is a partner establishment (see model.Affiliate). Admins manage a
// drag-orderable list of them — create/edit/delete/reorder — with owners and
// opening hours edited as repeatable rows and persisted as JSON. The logo +
// screenshot are picked from the shared image library (System → Images). A single
// shared Discord webhook (kept out of public settings) lets each affiliate be
// posted as an embed. All of this is admin-only, gated by permTeahouseAffiliates.

// affiliateWebhookSettingKey stores the single shared Discord webhook every
// affiliate posts to. Deliberately NOT in settingsKeys (server/settings.go), so it
// never leaks through the public GET /api/settings — read/written only through
// these permission-gated endpoints.
const affiliateWebhookSettingKey = "affiliate_webhook_url"

// handleAffiliatesList returns every affiliate, alphabetically by name.
//
//	Endpoint:  GET /api/affiliates
//	Auth:      admin, or a user granted teahouse-affiliates
//	Response:  {"affiliates": [...]}
func (s *Server) handleAffiliatesList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAffiliates) {
		return
	}
	affiliates, err := s.store.ListAffiliates()
	if err != nil {
		writeInternalError(w, "list affiliates", err)
		return
	}
	webhook, _ := s.store.GetSetting(affiliateWebhookSettingKey)
	writeJSON(w, http.StatusOK, model.AffiliatesResponse{Affiliates: affiliates, WebhookURL: webhook})
}

// affiliateWriteRequest is the JSON body for creating (POST /api/affiliates) or
// replacing (PUT /api/affiliates/{id}) an affiliate. The id comes from the path.
type affiliateWriteRequest struct {
	Name        string                `json:"name"`
	Owners      []string              `json:"owners"`
	Location    string                `json:"location"`
	Timezone    string                `json:"timezone"`
	Hours       []model.AffiliateHour `json:"hours"`
	Details     string                `json:"details"`
	Logo        string                `json:"logo"`
	Screenshot  string                `json:"screenshot"`
	EmbedColor  string                `json:"embed_color"`
	DiscordLink string                `json:"discord_link"`
	CarrdLink   string                `json:"carrd_link"`
}

// sanitizeOwners trims each owner name and drops blanks, preserving order. Blank
// repeater rows from the form are discarded so they aren't persisted.
func sanitizeOwners(in []string) []string {
	out := make([]string, 0, len(in))
	for _, o := range in {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

// sanitizeAffiliateHours trims each hours row and drops rows without a start time
// (an empty repeater row), preserving order. A row needs at least a start.
func sanitizeAffiliateHours(in []model.AffiliateHour) []model.AffiliateHour {
	out := make([]model.AffiliateHour, 0, len(in))
	for _, h := range in {
		start := strings.TrimSpace(h.Start)
		if start == "" {
			continue
		}
		out = append(out, model.AffiliateHour{
			Label: strings.TrimSpace(h.Label),
			Start: start,
			End:   strings.TrimSpace(h.End),
		})
	}
	return out
}

// affiliateFromRequest builds a sanitized model.Affiliate (sans ID) from a request.
func affiliateFromRequest(req affiliateWriteRequest, name string) *model.Affiliate {
	return &model.Affiliate{
		Name:        name,
		Owners:      sanitizeOwners(req.Owners),
		Location:    strings.TrimSpace(req.Location),
		Timezone:    strings.TrimSpace(req.Timezone),
		Hours:       sanitizeAffiliateHours(req.Hours),
		Details:     req.Details,
		Logo:        strings.TrimSpace(req.Logo),
		Screenshot:  strings.TrimSpace(req.Screenshot),
		EmbedColor:  strings.TrimSpace(req.EmbedColor),
		DiscordLink: strings.TrimSpace(req.DiscordLink),
		CarrdLink:   strings.TrimSpace(req.CarrdLink),
	}
}

// handleAffiliateCreate creates an affiliate.
//
//	Endpoint:  POST /api/affiliates
//	Auth:      admin, or a user granted teahouse-affiliates
//	Response:  201 {"affiliate": Affiliate}
func (s *Server) handleAffiliateCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAffiliates) {
		return
	}
	req, err := readJSON[affiliateWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Name is required")
		return
	}
	affiliate := affiliateFromRequest(req, name)
	id, err := s.store.CreateAffiliate(affiliate)
	if err != nil {
		writeInternalError(w, "create affiliate", err)
		return
	}
	affiliate.ID = id
	writeJSON(w, http.StatusCreated, model.AffiliateResponse{Affiliate: *affiliate})
}

// handleAffiliateUpdate replaces an affiliate.
//
//	Endpoint:  PUT /api/affiliates/{id}
//	Auth:      admin, or a user granted teahouse-affiliates
//	Response:  200 {"ok": true}
func (s *Server) handleAffiliateUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAffiliates) {
		return
	}
	id, ok := pathInt64(w, r, "id", "affiliate")
	if !ok {
		return
	}
	req, err := readJSON[affiliateWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Name is required")
		return
	}
	affiliate := affiliateFromRequest(req, name)
	affiliate.ID = id
	if err := s.store.UpdateAffiliate(affiliate); err != nil {
		writeInternalError(w, "update affiliate", err)
		return
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}

// handleAffiliateDelete deletes an affiliate.
//
//	Endpoint:  DELETE /api/affiliates/{id}
//	Auth:      admin, or a user granted teahouse-affiliates
//	Response:  204 No Content
func (s *Server) handleAffiliateDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAffiliates) {
		return
	}
	id, ok := pathInt64(w, r, "id", "affiliate")
	if !ok {
		return
	}
	if _, err := s.store.DeleteAffiliate(id); err != nil {
		writeInternalError(w, "delete affiliate", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// affiliateReorderRequest is the JSON body for POST /api/affiliates/reorder.
type affiliateReorderRequest struct {
	OrderedIDs []int64 `json:"ordered_ids"`
}

// handleAffiliatesReorder persists a new drag order (top-first ids).
//
//	Endpoint:  POST /api/affiliates/reorder
//	Auth:      admin, or a user granted teahouse-affiliates
//	Response:  200 {"ok": true}
func (s *Server) handleAffiliatesReorder(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAffiliates) {
		return
	}
	req, err := readJSON[affiliateReorderRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if err := s.store.BulkReorderAffiliates(req.OrderedIDs); err != nil {
		writeInternalError(w, "reorder affiliates", err)
		return
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}

// affiliateWebhookRequest is the JSON body for PUT /api/affiliates/webhook.
type affiliateWebhookRequest struct {
	WebhookURL string `json:"webhook_url"`
}

// handleAffiliateWebhookSet stores the single shared affiliates Discord webhook.
//
//	Endpoint:  PUT /api/affiliates/webhook
//	Auth:      admin, or a user granted teahouse-affiliates
//	Request:   {"webhook_url": "https://discord.com/api/webhooks/…"}  ('' clears)
//	Response:  200 {"webhook_url": "…"}
func (s *Server) handleAffiliateWebhookSet(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAffiliates) {
		return
	}
	req, err := readJSON[affiliateWebhookRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	webhook := strings.TrimSpace(req.WebhookURL)
	if webhook != "" && !isDiscordWebhookURL(webhook) {
		writeError(w, http.StatusBadRequest, "Discord webhook URLs must look like https://discord.com/api/webhooks/…")
		return
	}
	if err := s.store.SetSetting(affiliateWebhookSettingKey, webhook); err != nil {
		writeInternalError(w, "save affiliate webhook", err)
		return
	}
	writeJSON(w, http.StatusOK, model.AffiliateWebhookResponse{WebhookURL: webhook})
}

// handleAffiliatePost posts an affiliate to the shared Discord webhook as an embed.
//
//	Endpoint:  POST /api/affiliates/{id}/post
//	Auth:      admin, or a user granted teahouse-affiliates
//	Response:  200 {"affiliate": Affiliate}
//	           400 no webhook, 404 not found, 502 Discord post failed
func (s *Server) handleAffiliatePost(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAffiliates) {
		return
	}
	id, ok := pathInt64(w, r, "id", "affiliate")
	if !ok {
		return
	}
	affiliate, err := s.store.GetAffiliate(id)
	if err != nil {
		writeInternalError(w, "load affiliate for post", err)
		return
	}
	if affiliate == nil {
		writeError(w, http.StatusNotFound, "Affiliate not found")
		return
	}
	webhook, _ := s.store.GetSetting(affiliateWebhookSettingKey)
	if strings.TrimSpace(webhook) == "" {
		writeError(w, http.StatusBadRequest, "No Affiliates Discord webhook is configured. Set one on the Affiliates page first.")
		return
	}
	embed := buildAffiliateEmbed(*affiliate, s.siteBaseURL(r), time.Now())
	if err := postDiscordEmbed(webhook, embed); err != nil {
		writeError(w, http.StatusBadGateway, "Failed to post to Discord: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, model.AffiliateResponse{Affiliate: *affiliate})
}

// buildAffiliateEmbed renders an affiliate as a Discord embed: colour + name +
// markdown details, the logo as the thumbnail and the establishment screenshot as
// the large image, and — in order, each skipped when absent — the location
// (full-width), the opening hours (full-width, as Discord "Short Time" tokens
// <t:unix:t> so each viewer sees them in their own timezone), and finally the
// Discord + Carrd links as two side-by-side fields. baseURL (scheme://host) turns
// the root-relative logo/screenshot paths into absolute image URLs; now anchors
// the recurring wall-clock hours onto a concrete date for the timestamp tokens.
func buildAffiliateEmbed(a model.Affiliate, baseURL string, now time.Time) discordEmbed {
	b := newEmbed().title(a.Name).colorHex(a.EmbedColor)
	b.description(discordMarkdown(strings.TrimSpace(a.Details)))

	// Location first, full-width.
	b.field("📍 Location", strings.TrimSpace(a.Location), false)

	// Opening hours next, full-width — one line per row (days in the admin's order),
	// each time as a local-time token.
	if hours := formatAffiliateHours(a, now); hours != "" {
		b.field("🕒 Open Times", hours, false)
		b.footer("Times are displayed in your local time zone.")
	}

	// Discord + Carrd links last, side by side.
	if link := normalizeExternalURL(a.DiscordLink); link != "" {
		b.field("💬 Discord", "[Open Link]("+link+")", true)
	}
	if link := normalizeExternalURL(a.CarrdLink); link != "" {
		b.field("🔗 Carrd", "[Open Link]("+link+")", true)
	}

	b.thumbnail(absoluteAssetURL(baseURL, a.Logo))
	b.image(absoluteAssetURL(baseURL, a.Screenshot))
	return b.build()
}

// formatAffiliateHours renders an affiliate's opening hours as newline-separated
// lines of Discord "Short Time" timestamp tokens, anchored in the affiliate's
// timezone (falling back to UTC if it's blank/invalid). Returns "" when there are
// no usable rows.
func formatAffiliateHours(a model.Affiliate, now time.Time) string {
	if len(a.Hours) == 0 {
		return ""
	}
	loc, err := time.LoadLocation(strings.TrimSpace(a.Timezone))
	if err != nil || loc == nil {
		loc = time.UTC
	}
	lines := make([]string, 0, len(a.Hours))
	for _, h := range a.Hours {
		start, ok := hhmmToUnix(h.Start, loc, now)
		if !ok {
			continue
		}
		var line strings.Builder
		if label := strings.TrimSpace(h.Label); label != "" {
			line.WriteString("**" + label + "** — ")
		}
		line.WriteString(fmt.Sprintf("<t:%d:t>", start))
		if end, ok := hhmmToUnix(h.End, loc, now); ok {
			line.WriteString(fmt.Sprintf(" – <t:%d:t>", end))
		}
		lines = append(lines, line.String())
	}
	return strings.Join(lines, "\n")
}

// hhmmToUnix converts a "HH:MM" wall-clock time, interpreted in loc and anchored
// to now's calendar date, into a Unix timestamp. The date is irrelevant to a
// "Short Time" (t) token — only the time-of-day shows — but anchoring on "now"
// keeps the DST offset current.
func hhmmToUnix(hhmm string, loc *time.Location, now time.Time) (int64, bool) {
	hs, ms, found := strings.Cut(strings.TrimSpace(hhmm), ":")
	if !found {
		return 0, false
	}
	h, err1 := strconv.Atoi(hs)
	m, err2 := strconv.Atoi(ms)
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	ref := now.In(loc)
	return time.Date(ref.Year(), ref.Month(), ref.Day(), h, m, 0, 0, loc).Unix(), true
}

// absoluteAssetURL turns a root-relative asset path (e.g. images/affiliate_logos/x.png)
// into an absolute URL under baseURL. Already-absolute http(s) URLs pass through;
// blanks stay blank (the embed builder then skips the image).
func absoluteAssetURL(baseURL, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || isHTTPURL(path) {
		return path
	}
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimPrefix(path, "/")
}

// normalizeExternalURL trims a user-entered link and prepends https:// when it has
// no scheme, so it's a valid target for an embed masked link. Blank stays blank.
func normalizeExternalURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" || isHTTPURL(u) {
		return u
	}
	return "https://" + u
}
