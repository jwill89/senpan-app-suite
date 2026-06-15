package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"app-suite/internal/model"
)

// announcementSchedulerInterval is how often the background scheduler checks for
// due announcement posts. Announcements post within one interval of their time.
const announcementSchedulerInterval = 30 * time.Second

// validScheduleKinds is the set of accepted schedule kinds (empty = unscheduled).
var validScheduleKinds = map[string]bool{"": true, "once": true, "daily": true, "weekly": true, "monthly": true}

// ── Announcement types ──────────────────────────────────────────────────────

// handleAnnouncementTypesList returns all announcement types.
//
//	Endpoint:  GET /api/announcement-types
//	Auth:      admin
//	Response:  {"types": [...]}
func (s *Server) handleAnnouncementTypesList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAnnounce) {
		return
	}
	types, err := s.store.ListAnnouncementTypes()
	if err != nil {
		writeInternalError(w, "list announcement types", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"types": types})
}

// announcementTypeRequest is the JSON body for POST /api/announcement-types.
type announcementTypeRequest struct {
	Action     string `json:"action"`
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	WebhookURL string `json:"webhook_url"`
}

// handleAnnouncementTypesAction creates, updates, or deletes an announcement type.
//
//	Endpoint:  POST /api/announcement-types
//	Auth:      admin
//	Request:   {"action": "create"|"update"|"delete", ...}
func (s *Server) handleAnnouncementTypesAction(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAnnounce) {
		return
	}
	req, err := readJSON[announcementTypeRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Action {
	case "create":
		name := strings.TrimSpace(req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "Type name is required")
			return
		}
		id, err := s.store.CreateAnnouncementType(name, strings.TrimSpace(req.WebhookURL))
		if err != nil {
			writeInternalError(w, "create announcement type", err)
			return
		}
		saved, _ := s.store.GetAnnouncementType(id)
		writeJSON(w, http.StatusCreated, map[string]any{"type": saved})

	case "update":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Type id is required")
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "Type name is required")
			return
		}
		if err := s.store.UpdateAnnouncementType(req.ID, name, strings.TrimSpace(req.WebhookURL)); err != nil {
			writeInternalError(w, "update announcement type", err)
			return
		}
		saved, _ := s.store.GetAnnouncementType(req.ID)
		writeJSON(w, http.StatusOK, map[string]any{"type": saved})

	case "delete":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Type id is required")
			return
		}
		count, err := s.store.CountAnnouncementsByType(req.ID)
		if err != nil {
			writeInternalError(w, "count announcements by type", err)
			return
		}
		if count > 0 {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("This type is used by %d announcement(s). Reassign or delete them first.", count))
			return
		}
		deleted, err := s.store.DeleteAnnouncementType(req.ID)
		if err != nil {
			writeInternalError(w, "delete announcement type", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: create, update, delete")
	}
}

// ── Announcements ───────────────────────────────────────────────────────────

// handleAnnouncementsList returns all announcements (with their type name).
//
//	Endpoint:  GET /api/announcements
//	Auth:      admin
//	Response:  {"announcements": [...]}
func (s *Server) handleAnnouncementsList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAnnounce) {
		return
	}
	items, err := s.store.ListAnnouncements()
	if err != nil {
		writeInternalError(w, "list announcements", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"announcements": items})
}

// announcementRequest is the JSON body for POST /api/announcements.
type announcementRequest struct {
	Action       string             `json:"action"`
	ID           int64              `json:"id"`
	Announcement model.Announcement `json:"announcement"`
}

// handleAnnouncementsAction creates, updates, deletes, sends, or skips an
// announcement.
//
//	Endpoint:  POST /api/announcements
//	Auth:      admin
//	Request:   {"action": "create"|"update"|"delete"|"send_now"|"skip_next", ...}
func (s *Server) handleAnnouncementsAction(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAnnounce) {
		return
	}
	req, err := readJSON[announcementRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Action {
	case "create":
		a := req.Announcement
		if !s.validateAndResolveAnnouncement(w, &a) {
			return
		}
		id, err := s.store.CreateAnnouncement(&a)
		if err != nil {
			writeInternalError(w, "create announcement", err)
			return
		}
		saved, _ := s.store.GetAnnouncement(id)
		writeJSON(w, http.StatusCreated, map[string]any{"announcement": saved})

	case "update":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Announcement id is required")
			return
		}
		existing, err := s.store.GetAnnouncement(req.ID)
		if err != nil {
			writeInternalError(w, "load announcement for update", err)
			return
		}
		if existing == nil {
			writeError(w, http.StatusNotFound, "Announcement not found")
			return
		}
		a := req.Announcement
		if !s.validateAndResolveAnnouncement(w, &a) {
			return
		}
		a.ID = req.ID
		if existing.Image != a.Image {
			s.removeUploadedAnnouncementImageIfUnused(existing.Image, a.ID)
		}
		if err := s.store.UpdateAnnouncement(&a); err != nil {
			writeInternalError(w, "update announcement", err)
			return
		}
		saved, _ := s.store.GetAnnouncement(a.ID)
		writeJSON(w, http.StatusOK, map[string]any{"announcement": saved})

	case "delete":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Announcement id is required")
			return
		}
		if existing, err := s.store.GetAnnouncement(req.ID); err == nil && existing != nil {
			s.removeUploadedAnnouncementImageIfUnused(existing.Image, existing.ID)
		}
		deleted, err := s.store.DeleteAnnouncement(req.ID)
		if err != nil {
			writeInternalError(w, "delete announcement", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})

	case "send_now":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Announcement id is required")
			return
		}
		a, err := s.store.GetAnnouncement(req.ID)
		if err != nil {
			writeInternalError(w, "load announcement for send", err)
			return
		}
		if a == nil {
			writeError(w, http.StatusNotFound, "Announcement not found")
			return
		}
		typ, _ := s.store.GetAnnouncementType(a.TypeID)
		if typ == nil || strings.TrimSpace(typ.WebhookURL) == "" {
			writeError(w, http.StatusBadRequest, "This announcement's type has no Discord webhook configured.")
			return
		}
		if err := postDiscordWebhook(typ.WebhookURL, buildAnnouncementMessage(*a)); err != nil {
			writeError(w, http.StatusBadGateway, "Failed to post to Discord: "+err.Error())
			return
		}
		if err := s.store.TouchAnnouncementPosted(a.ID); err != nil {
			writeInternalError(w, "stamp announcement posted", err)
			return
		}
		saved, _ := s.store.GetAnnouncement(a.ID)
		writeJSON(w, http.StatusOK, map[string]any{"announcement": saved})

	case "skip_next":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Announcement id is required")
			return
		}
		a, err := s.store.GetAnnouncement(req.ID)
		if err != nil {
			writeInternalError(w, "load announcement for skip", err)
			return
		}
		if a == nil {
			writeError(w, http.StatusNotFound, "Announcement not found")
			return
		}
		if a.ScheduleKind == "" || a.NextPostAt == "" {
			writeError(w, http.StatusBadRequest, "This announcement isn't scheduled.")
			return
		}
		if err := s.store.SetAnnouncementSkip(a.ID, true); err != nil {
			writeInternalError(w, "skip announcement", err)
			return
		}
		saved, _ := s.store.GetAnnouncement(a.ID)
		writeJSON(w, http.StatusOK, map[string]any{"announcement": saved})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: create, update, delete, send_now, skip_next")
	}
}

// validateAndResolveAnnouncement validates required fields, confirms the type
// exists, and resolves every wall-clock time against the announcement's single
// IANA timezone — computing the absolute UTC instants (start_at/end_at and, for a
// schedule, next_post_at). On failure it writes the error response, returns false.
func (s *Server) validateAndResolveAnnouncement(w http.ResponseWriter, a *model.Announcement) bool {
	a.Title = strings.TrimSpace(a.Title)
	if a.Title == "" {
		writeError(w, http.StatusBadRequest, "Title is required")
		return false
	}
	a.Details = strings.TrimSpace(a.Details)
	if a.Details == "" {
		writeError(w, http.StatusBadRequest, "Details are required")
		return false
	}
	if a.TypeID <= 0 {
		writeError(w, http.StatusBadRequest, "An announcement type is required")
		return false
	}
	typ, err := s.store.GetAnnouncementType(a.TypeID)
	if err != nil {
		writeInternalError(w, "load announcement type", err)
		return false
	}
	if typ == nil {
		writeError(w, http.StatusBadRequest, "Invalid announcement type")
		return false
	}
	a.Image = strings.TrimSpace(a.Image)
	a.Color = strings.TrimSpace(a.Color)
	a.StartLocal = strings.TrimSpace(a.StartLocal)
	a.EndLocal = strings.TrimSpace(a.EndLocal)
	a.OnceLocal = strings.TrimSpace(a.OnceLocal)
	a.Buttons = sanitizeAnnouncementButtons(a.Buttons)

	a.ScheduleKind = strings.TrimSpace(a.ScheduleKind)
	if !validScheduleKinds[a.ScheduleKind] {
		writeError(w, http.StatusBadRequest, "Invalid schedule kind")
		return false
	}
	a.SkipNext = false

	// One timezone anchors every wall-clock time. It's required whenever any time
	// is present (the event window or a schedule); otherwise it's irrelevant.
	hasTime := a.StartLocal != "" || a.EndLocal != "" || a.ScheduleKind != ""
	a.Timezone = strings.TrimSpace(a.Timezone)
	loc := time.UTC
	if hasTime {
		if a.Timezone == "" {
			writeError(w, http.StatusBadRequest, "A timezone is required when an announcement has any times")
			return false
		}
		l, err := time.LoadLocation(a.Timezone)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid timezone")
			return false
		}
		loc = l
	}

	// Resolve the optional event window (wall-clock in loc → UTC instant).
	if a.StartAt, err = resolveLocalInstant(a.StartLocal, loc); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid start date/time")
		return false
	}
	if a.EndAt, err = resolveLocalInstant(a.EndLocal, loc); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid end date/time")
		return false
	}

	switch a.ScheduleKind {
	case "":
		// Unscheduled — manual posting only.
		a.NextPostAt = ""
		a.Active = false
	case "once":
		if a.OnceLocal == "" {
			writeError(w, http.StatusBadRequest, "A date & time is required for a one-time schedule")
			return false
		}
		next, err := resolveLocalInstant(a.OnceLocal, loc)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid one-time date/time")
			return false
		}
		a.NextPostAt = next
		a.Active = true
	default:
		// Recurring — resolved in loc by nextAnnouncementOccurrence (DST-safe).
		next := nextAnnouncementOccurrence(*a, time.Now())
		if next == "" {
			writeError(w, http.StatusBadRequest, "The recurring schedule is incomplete (pick day(s) and a time)")
			return false
		}
		a.NextPostAt = next
		a.Active = true
	}
	return true
}

// resolveLocalInstant converts a wall-clock "2006-01-02T15:04" value in loc to a
// UTC RFC-3339 string. An empty value resolves to "" (no error).
func resolveLocalInstant(value string, loc *time.Location) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	t, err := parseLocalInZone(value, loc)
	if err != nil {
		return "", err
	}
	return t.UTC().Format(time.RFC3339), nil
}

// ── Embed ───────────────────────────────────────────────────────────────────

// maxAnnouncementButtons caps how many link buttons an announcement can carry
// (Discord allows five buttons in a single action row).
const maxAnnouncementButtons = 5

// sanitizeAnnouncementButtons trims each button's fields and keeps only valid ones
// (a non-empty label and an http(s) URL), capping the result at five. Returns nil
// when nothing valid remains so the stored value is an empty array.
func sanitizeAnnouncementButtons(buttons []model.AnnouncementButton) []model.AnnouncementButton {
	if len(buttons) == 0 {
		return nil
	}
	out := make([]model.AnnouncementButton, 0, len(buttons))
	for _, b := range buttons {
		label := strings.TrimSpace(b.Label)
		url := strings.TrimSpace(b.URL)
		if label == "" || !isHTTPURL(url) {
			continue
		}
		out = append(out, model.AnnouncementButton{
			Label: label,
			Emoji: strings.TrimSpace(b.Emoji),
			URL:   url,
		})
		if len(out) >= maxAnnouncementButtons {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// buildAnnouncementMessage assembles the full Discord webhook payload for an
// announcement: its embed plus an optional action row of link buttons.
func buildAnnouncementMessage(a model.Announcement) discordWebhookPayload {
	return discordWebhookPayload{
		Embeds:     []discordEmbed{buildAnnouncementEmbed(a)},
		Components: announcementComponents(a),
	}
}

// announcementComponents builds the announcement's link-button action row (up to
// five), or nil when it has no valid buttons.
func announcementComponents(a model.Announcement) []discordComponent {
	if len(a.Buttons) == 0 {
		return nil
	}
	triples := make([]struct{ Label, Emoji, URL string }, 0, len(a.Buttons))
	for _, b := range a.Buttons {
		triples = append(triples, struct{ Label, Emoji, URL string }{b.Label, b.Emoji, b.URL})
	}
	return linkButtonRow(triples)
}

// buildAnnouncementEmbed renders an announcement as a Discord embed. When times
// are set they render first, as a single inline field using Discord <t:…> tokens
// (long "F" start, short "t" end) so each viewer sees their own zone. The markdown
// details follow as a full-width field with no visible heading; an image renders
// full-width at the bottom. The accent colour comes from the announcement (brand
// default if unset).
func buildAnnouncementEmbed(a model.Announcement) discordEmbed {
	b := newEmbed().
		title(a.Title).
		colorHex(a.Color)

	hasTime := false
	if startT, err := time.Parse(time.RFC3339, a.StartAt); err == nil {
		value := fmt.Sprintf("<t:%d:F>", startT.Unix())
		if endT, err := time.Parse(time.RFC3339, a.EndAt); err == nil {
			value += fmt.Sprintf(" to <t:%d:t>", endT.Unix())
		}
		b.field("🗓️ When", value, true)
		hasTime = true
	}
	// Details below the time, headingless (zero-width-space name) and full-width.
	b.field(embedNoHeading, a.Details, false)
	b.image(a.Image)
	if hasTime {
		b.footer("Times shown in your local time zone.")
	}
	return b.build()
}

// ── Recurrence ──────────────────────────────────────────────────────────────

// nextAnnouncementOccurrence returns the next scheduled instant strictly after
// `after`, as a UTC RFC-3339 string, or "" when there is no further occurrence.
// Recurring times are wall-clock values in the announcement's IANA Timezone, so
// the calendar math runs in that zone (Go normalizes across DST) and the result
// is converted to UTC for storage. An empty/invalid Timezone falls back to UTC.
func nextAnnouncementOccurrence(a model.Announcement, after time.Time) string {
	loc := time.UTC
	if tz := strings.TrimSpace(a.Timezone); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	after = after.In(loc)
	h, m := a.ScheduleMinutes/60, a.ScheduleMinutes%60

	switch a.ScheduleKind {
	case "daily":
		cand := time.Date(after.Year(), after.Month(), after.Day(), h, m, 0, 0, loc)
		for !cand.After(after) {
			cand = cand.AddDate(0, 0, 1)
		}
		return cand.UTC().Format(time.RFC3339)

	case "weekly":
		days := parseWeekdaySet(a.ScheduleWeekdays)
		if len(days) == 0 {
			return ""
		}
		// Scan up to 8 days so we also catch "same weekday, but later today".
		for i := 0; i <= 7; i++ {
			d := after.AddDate(0, 0, i)
			if days[int(d.Weekday())] {
				cand := time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, loc)
				if cand.After(after) {
					return cand.UTC().Format(time.RFC3339)
				}
			}
		}
		return ""

	case "monthly":
		wd, ok := firstWeekday(a.ScheduleWeekdays)
		if !ok {
			return ""
		}
		// Walk forward month by month until an occurrence lands after `after`.
		for i := 0; i < 13; i++ {
			base := time.Date(after.Year(), after.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, i, 0)
			cand := nthWeekdayOfMonth(base.Year(), base.Month(), wd, a.ScheduleWeekOfMonth, h, m, loc)
			if !cand.IsZero() && cand.After(after) {
				return cand.UTC().Format(time.RFC3339)
			}
		}
		return ""
	}
	return ""
}

// parseWeekdaySet parses a CSV of UTC weekday numbers (0=Sun..6=Sat) into a set.
func parseWeekdaySet(csv string) map[int]bool {
	out := map[int]bool{}
	for _, part := range strings.Split(csv, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil && n >= 0 && n <= 6 {
			out[n] = true
		}
	}
	return out
}

// firstWeekday returns the first weekday in a CSV (for the monthly rule).
func firstWeekday(csv string) (time.Weekday, bool) {
	for _, part := range strings.Split(csv, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil && n >= 0 && n <= 6 {
			return time.Weekday(n), true
		}
	}
	return 0, false
}

// nthWeekdayOfMonth returns the nth (1..5, or -1 = last) `wd` weekday of the
// given month at h:m in loc, or the zero time if that occurrence doesn't exist
// (e.g. a 5th Friday in a month with only four).
func nthWeekdayOfMonth(year int, month time.Month, wd time.Weekday, n, h, m int, loc *time.Location) time.Time {
	first := time.Date(year, month, 1, h, m, 0, 0, loc)
	offset := (int(wd) - int(first.Weekday()) + 7) % 7
	firstDay := 1 + offset // day-of-month of the first matching weekday
	daysInMonth := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()

	if n == -1 {
		last := firstDay
		for last+7 <= daysInMonth {
			last += 7
		}
		return time.Date(year, month, last, h, m, 0, 0, loc)
	}
	day := firstDay + (n-1)*7
	if day < 1 || day > daysInMonth {
		return time.Time{}
	}
	return time.Date(year, month, day, h, m, 0, 0, loc)
}

// ── Image upload (upload + pick existing) ───────────────────────────────────

// announcementImageDir is the upload directory for announcement images.
func (s *Server) announcementImageDir() string {
	return filepath.Join(s.webRoot, filepath.FromSlash(announcementImageRelDir))
}

// handleAnnouncementUpload stores a multipart announcement image under
// <webRoot>/images/announcements and returns its full URL.
//
//	Endpoint:  POST /api/announcements/upload
//	Auth:      admin
//	Response:  {"url": "https://host/images/announcements/announcement_....ext"}
func (s *Server) handleAnnouncementUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAnnounce) {
		return
	}
	s.saveSingleImageUpload(w, r, announcementImageRelDir, "announcement")
}

// handleAnnouncementImages lists existing announcement images (newest first) as
// full URLs so the admin can reuse one instead of uploading a duplicate.
//
//	Endpoint:  GET /api/announcements/images
//	Auth:      admin
//	Response:  {"images": ["https://host/images/announcements/....png", ...]}
func (s *Server) handleAnnouncementImages(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAnnounce) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"images": s.listUploadedImageURLs(r, announcementImageRelDir)})
}

// removeUploadedAnnouncementImageIfUnused deletes an uploaded announcement image,
// but only when it lives in the announcements upload directory and no other
// announcement still references it (images are intentionally reusable).
func (s *Server) removeUploadedAnnouncementImageIfUnused(imageURL string, excludeID int64) {
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" || !strings.Contains(imageURL, "/images/announcements/") {
		return
	}
	items, err := s.store.ListAnnouncements()
	if err != nil {
		return // be conservative on error: keep the file
	}
	for _, it := range items {
		if it.ID != excludeID && strings.TrimSpace(it.Image) == imageURL {
			return
		}
	}
	name := imageURL[strings.LastIndex(imageURL, "/")+1:]
	if name == "" || strings.ContainsAny(name, `/\`) {
		return
	}
	uploadDir, err := filepath.Abs(s.announcementImageDir())
	if err != nil {
		return
	}
	target, err := filepath.Abs(filepath.Join(s.announcementImageDir(), filepath.Base(name)))
	if err != nil || !strings.HasPrefix(target, uploadDir+string(os.PathSeparator)) {
		return
	}
	_ = os.Remove(target)
}

// ── Scheduler ───────────────────────────────────────────────────────────────

// RunAnnouncementScheduler posts due announcements to their type's webhook on a
// fixed interval until ctx is cancelled. Safe to call in a goroutine.
func (s *Server) RunAnnouncementScheduler(ctx context.Context) {
	runScheduler(ctx, announcementSchedulerInterval, s.postDueAnnouncements)
}

// postDueAnnouncements posts every announcement whose scheduled time has arrived.
// A "skip next" marker advances the cursor without posting; an announcement whose
// type has no webhook is left pending; a failed post is retried next tick.
func (s *Server) postDueAnnouncements() {
	due, err := s.store.DueAnnouncements(time.Now())
	if err != nil {
		slog.Error("announcement scheduler: load due", "error", err)
		return
	}
	for _, a := range due {
		if a.SkipNext {
			next, active := s.advanceCursor(a)
			if err := s.store.AdvanceAnnouncement(a.ID, next, active, false); err != nil {
				slog.Error("announcement scheduler: clear skip", "id", a.ID, "error", err)
			}
			continue
		}
		typ, _ := s.store.GetAnnouncementType(a.TypeID)
		if typ == nil || strings.TrimSpace(typ.WebhookURL) == "" {
			continue // no webhook yet; try again next tick
		}
		if err := postDiscordWebhook(typ.WebhookURL, buildAnnouncementMessage(a)); err != nil {
			slog.Error("announcement scheduler: post", "id", a.ID, "error", err)
			continue // leave pending; retry next tick
		}
		next, active := s.advanceCursor(a)
		if err := s.store.MarkAnnouncementPosted(a.ID, next, active); err != nil {
			slog.Error("announcement scheduler: mark posted", "id", a.ID, "error", err)
		} else {
			slog.Info("posted scheduled announcement", "id", a.ID, "title", a.Title)
		}
	}
}

// advanceCursor computes the next schedule cursor for an announcement after its
// current next_post_at fires. Recurring kinds roll forward; a one-time schedule
// has no further occurrence (returns "", false → deactivated).
func (s *Server) advanceCursor(a model.Announcement) (nextPostAt string, active bool) {
	if a.ScheduleKind == "" || a.ScheduleKind == "once" {
		return "", false
	}
	after := time.Now()
	if t, err := time.Parse(time.RFC3339, a.NextPostAt); err == nil {
		after = t
	}
	next := nextAnnouncementOccurrence(a, after)
	return next, next != ""
}
