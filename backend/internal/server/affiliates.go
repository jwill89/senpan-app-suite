package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"app-suite/internal/discord"
	"app-suite/internal/model"
)

// -- Affiliates admin (list + CRUD + drag order + post to Discord) ------------
//
// An affiliate is a partner establishment (see model.Affiliate). Admins manage a
// drag-orderable list of them - create/edit/delete/reorder - with owners and
// opening hours edited as repeatable rows and persisted as JSON. The logo +
// screenshot are picked from the shared image library (System -> Images). The bot
// posts each affiliate as an embed to the Affiliates channel, edited in place as the
// affiliate changes when the destination is set to (see discordqueue.go). All of this
// is admin-only, gated by permTeahouseAffiliates.

// handleAffiliatesList returns every affiliate, alphabetically by name.
//
//	Endpoint:  GET /api/affiliates
//	Auth:      admin, or a user granted teahouse-affiliates, festival-map or
//	           festival-stamp-rally (the latter two only to name a stall's operator)
//	Response:  model.AffiliatesResponse (destination + posts for the affiliates page only)
func (s *Server) handleAffiliatesList(w http.ResponseWriter, r *http.Request) {
	// The festival map and stamp rally editors both pick an affiliate to name who
	// runs a pitch, so they need to READ this list - gating it on the affiliates
	// page alone left those grantees with an empty operator select and no way to
	// finish the form. They get the names only: where affiliates post in Discord
	// belongs to the affiliates page.
	u := s.requireAnyPermission(w, r, permTeahouseAffiliates, permFestivalMap, permFestivalStampRally)
	if u == nil {
		return
	}
	affiliates, err := s.store.ListAffiliates()
	if err != nil {
		writeInternalError(w, "list affiliates", err)
		return
	}
	res := model.AffiliatesResponse{Affiliates: affiliates}
	if u.IsAdmin || userHasPermission(u, permTeahouseAffiliates) {
		dest, err := s.store.GetDiscordDestination(destAffiliates)
		if err != nil {
			writeInternalError(w, "read affiliates destination", err)
			return
		}
		res.Destination = &dest
		posts := s.discordPostStates(postKindAffiliate)
		for i := range res.Affiliates {
			res.Affiliates[i].DiscordPost = posts[res.Affiliates[i].ID]
		}
	}
	writeJSON(w, http.StatusOK, res)
}

// affiliateWriteRequest is the JSON body for creating (POST /api/affiliates) or
// replacing (PUT /api/affiliates/{id}) an affiliate. The id comes from the path.
type affiliateWriteRequest struct {
	Name        string                `json:"name"`
	Subtitle    string                `json:"subtitle"`
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
		Name:       name,
		Subtitle:   strings.TrimSpace(req.Subtitle),
		Owners:     sanitizeOwners(req.Owners),
		Location:   strings.TrimSpace(req.Location),
		Timezone:   strings.TrimSpace(req.Timezone),
		Hours:      sanitizeAffiliateHours(req.Hours),
		Details:    req.Details,
		Logo:       strings.TrimSpace(req.Logo),
		Screenshot: strings.TrimSpace(req.Screenshot),
		EmbedColor: strings.TrimSpace(req.EmbedColor),
		// Normalize rather than merely trim: these two are rendered as `href` on the
		// PUBLIC festival map, and normalizeExternalURL turns anything that is not
		// already an http(s) URL into "https://<what they typed>" - so a stored
		// "javascript:..." becomes an inert https link instead of a live one. The
		// embed builder already ran these through it; the stored value did not.
		DiscordLink: normalizeExternalURL(req.DiscordLink),
		CarrdLink:   normalizeExternalURL(req.CarrdLink),
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
	s.queueDiscordEdit(postKindAffiliate, id)
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}

// handleAffiliateDelete deletes an affiliate, and takes its Discord post down.
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
	affiliate, err := s.store.GetAffiliate(id)
	if err != nil {
		writeInternalError(w, "load affiliate for delete", err)
		return
	}
	if _, err := s.store.DeleteAffiliate(id); err != nil {
		writeInternalError(w, "delete affiliate", err)
		return
	}
	if affiliate != nil {
		ctx, cancel := detachedDiscordContext(r, discordWriteTimeout)
		s.forgetDiscordItem(ctx, affiliateItems, id, affiliate.Name)
		cancel()
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

// handleAffiliatePost posts an affiliate to the Affiliates channel - or, when the
// channel is set to edit in place and the affiliate already has a post there, updates
// that post.
//
//	Endpoint:  POST /api/affiliates/{id}/post
//	Auth:      admin, or a user granted teahouse-affiliates
//	Response:  model.DiscordPostResult
//	           400 no channel picked, 404 not found, 409 Discord off, 502 Discord refused
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
	ctx, cancel := detachedDiscordContext(r, discordWriteTimeout)
	defer cancel()
	res, err := s.postDiscordItem(ctx, affiliateItems, id)
	if err != nil {
		writeDiscordFailure(w, fmt.Sprintf("post affiliate %d", id), affiliateItems.where, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleAffiliateTakeDown deletes an affiliate's post from Discord.
//
//	Endpoint:  DELETE /api/affiliates/{id}/post
//	Auth:      admin, or a user granted teahouse-affiliates
//	Response:  model.DiscordPostResult
func (s *Server) handleAffiliateTakeDown(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAffiliates) {
		return
	}
	id, ok := pathInt64(w, r, "id", "affiliate")
	if !ok {
		return
	}
	affiliate, err := s.store.GetAffiliate(id)
	if err != nil {
		writeInternalError(w, "load affiliate for take down", err)
		return
	}
	if affiliate == nil {
		writeError(w, http.StatusNotFound, "Affiliate not found")
		return
	}
	ctx, cancel := detachedDiscordContext(r, discordWriteTimeout)
	defer cancel()
	res, err := s.takeDownDiscordItem(ctx, affiliateItems, id, affiliate.Name)
	if err != nil {
		writeDiscordFailure(w, fmt.Sprintf("take down affiliate %d", id), affiliateItems.where, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// buildAffiliateEmbed renders an affiliate as a Discord embed: colour + name +
// markdown details, the logo as the thumbnail and the establishment screenshot as
// the large image, and - in order, each skipped when absent - the location
// (full-width), the opening hours (full-width, as Discord "Short Time" tokens
// <t:unix:t> so each viewer sees them in their own timezone), and finally the
// Discord + Carrd links as two side-by-side fields. baseURL (the public site URL)
// turns the root-relative logo/screenshot paths into absolute image URLs; now anchors
// the recurring wall-clock hours onto a concrete date for the timestamp tokens.
func buildAffiliateEmbed(a model.Affiliate, baseURL string, now time.Time) discord.Embed {
	b := discord.NewEmbed().Title(a.Name).ColorHex(a.EmbedColor)
	b.Description(discordMarkdown(strings.TrimSpace(a.Details)))

	// Location first, full-width.
	b.Field("📍 Location", strings.TrimSpace(a.Location), false)

	// Opening hours next, full-width - one line per row (days in the admin's order),
	// each time as a local-time token.
	if hours := formatAffiliateHours(a, now); hours != "" {
		b.Field("🕒 Open Times", hours, false)
		b.Footer("Times are displayed in your local time zone.")
	}

	// Discord + Carrd links last, side by side.
	if link := normalizeExternalURL(a.DiscordLink); link != "" {
		b.Field("💬 Discord", "[Open Link]("+link+")", true)
	}
	if link := normalizeExternalURL(a.CarrdLink); link != "" {
		b.Field("🔗 Carrd", "[Open Link]("+link+")", true)
	}

	b.Thumbnail(absoluteAssetURL(baseURL, a.Logo))
	b.Image(absoluteAssetURL(baseURL, a.Screenshot))
	return b.Build()
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
			line.WriteString("**" + label + "** - ")
		}
		fmt.Fprintf(&line, "<t:%d:t>", start)
		if end, ok := hhmmToUnix(h.End, loc, now); ok {
			fmt.Fprintf(&line, " - <t:%d:t>", end)
		}
		lines = append(lines, line.String())
	}
	return strings.Join(lines, "\n")
}

// hhmmToUnix converts a "HH:MM" wall-clock time, interpreted in loc and anchored
// to now's calendar date, into a Unix timestamp. The date is irrelevant to a
// "Short Time" (t) token - only the time-of-day shows - but anchoring on "now"
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
	if path == "" || discord.IsHTTPURL(path) {
		return path
	}
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimPrefix(path, "/")
}

// normalizeExternalURL trims a user-entered link and prepends https:// when it has
// no scheme, so it's a valid target for an embed masked link. Blank stays blank.
func normalizeExternalURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" || discord.IsHTTPURL(u) {
		return u
	}
	return "https://" + u
}
