package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"app-suite/internal/model"
)

// eventSchedulerInterval is how often the background scheduler checks for due
// event posts. Events post within one interval of their scheduled time.
const eventSchedulerInterval = 30 * time.Second

// eventTimeLayouts are accepted wall-clock formats from an <input type=
// "datetime-local"> (seconds optional).
var eventTimeLayouts = []string{"2006-01-02T15:04", "2006-01-02T15:04:05"}

// ── Handlers ────────────────────────────────────────────────────────────────

// handleBookClubEventsList returns all scheduled events for a book club.
//
//	Endpoint:  GET /api/bookclub/events?club=yaoi
//	Auth:      admin, or the book club's page permission
//	Response:  {"events": [...]}
func (s *Server) handleBookClubEventsList(w http.ResponseWriter, r *http.Request) {
	club := strings.TrimSpace(r.URL.Query().Get("club"))
	if club == "" {
		club = defaultClubSlug
	}
	if !s.requirePermission(w, r, bookClubPerm(club)) {
		return
	}
	events, err := s.store.ListBookClubEvents(club)
	if err != nil {
		writeInternalError(w, "list book club events", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

// bookClubEventRequest is the JSON body for POST /api/bookclub/events.
type bookClubEventRequest struct {
	Action   string              `json:"action"`
	ID       int64               `json:"id"`
	ClubSlug string              `json:"club_slug"`
	Event    model.BookClubEvent `json:"event"`
}

// handleBookClubEventsAction creates, updates, deletes, or immediately posts a
// book club event.
//
//	Endpoint:  POST /api/bookclub/events
//	Auth:      admin, or the book club's page permission
//	Request:   {"action": "create"|"update"|"delete"|"post_now", ...}
func (s *Server) handleBookClubEventsAction(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	req, err := readJSON[bookClubEventRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Resolve the owning book club to permission-check per club. create carries
	// the slug in the body; the id-based actions derive it from the record.
	club := strings.TrimSpace(req.ClubSlug)
	if req.Action == "update" || req.Action == "delete" || req.Action == "post_now" {
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Event id is required")
			return
		}
		existing, err := s.store.GetBookClubEvent(req.ID)
		if err != nil {
			writeInternalError(w, "load event", err)
			return
		}
		if existing == nil {
			writeError(w, http.StatusNotFound, "Event not found")
			return
		}
		club = existing.ClubSlug
	}
	if club == "" {
		club = defaultClubSlug
	}
	if !s.requirePermission(w, r, bookClubPerm(club)) {
		return
	}

	switch req.Action {
	case "create":
		ev := req.Event
		if !s.validateAndResolveEvent(w, &ev) {
			return
		}
		ev.ClubSlug = strings.TrimSpace(req.ClubSlug)
		if ev.ClubSlug == "" {
			ev.ClubSlug = defaultClubSlug
		}
		id, err := s.store.CreateBookClubEvent(&ev)
		if err != nil {
			writeInternalError(w, "create book club event", err)
			return
		}
		saved, _ := s.store.GetBookClubEvent(id)
		writeJSON(w, http.StatusCreated, map[string]any{"event": saved})

	case "update":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Event id is required")
			return
		}
		existing, err := s.store.GetBookClubEvent(req.ID)
		if err != nil {
			writeInternalError(w, "load event for update", err)
			return
		}
		if existing == nil {
			writeError(w, http.StatusNotFound, "Event not found")
			return
		}
		ev := req.Event
		if !s.validateAndResolveEvent(w, &ev) {
			return
		}
		ev.ID = req.ID
		// Replacing the image? Remove the old uploaded file it's no longer using.
		if existing.Image != ev.Image {
			s.removeUploadedEventImageIfUnused(existing.Image, ev.ID)
		}
		if err := s.store.UpdateBookClubEvent(&ev); err != nil {
			writeInternalError(w, "update book club event", err)
			return
		}
		saved, _ := s.store.GetBookClubEvent(ev.ID)
		writeJSON(w, http.StatusOK, map[string]any{"event": saved})

	case "delete":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Event id is required")
			return
		}
		if existing, err := s.store.GetBookClubEvent(req.ID); err == nil && existing != nil {
			s.removeUploadedEventImageIfUnused(existing.Image, existing.ID)
		}
		deleted, err := s.store.DeleteBookClubEvent(req.ID)
		if err != nil {
			writeInternalError(w, "delete book club event", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})

	case "post_now":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Event id is required")
			return
		}
		ev, err := s.store.GetBookClubEvent(req.ID)
		if err != nil {
			writeInternalError(w, "load event for post", err)
			return
		}
		if ev == nil {
			writeError(w, http.StatusNotFound, "Event not found")
			return
		}
		webhook, _ := s.store.GetSetting(eventsWebhookSettingKey(ev.ClubSlug))
		webhook = strings.TrimSpace(webhook)
		if webhook == "" {
			writeError(w, http.StatusBadRequest, "No events webhook configured for this book club. Set it under System → Settings.")
			return
		}
		if err := postDiscordEmbed(webhook, buildEventEmbed(*ev)); err != nil {
			writeError(w, http.StatusBadGateway, "Failed to post to Discord: "+err.Error())
			return
		}
		if err := s.store.MarkBookClubEventPosted(ev.ID); err != nil {
			writeInternalError(w, "mark event posted", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"posted": true})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: create, update, delete, post_now")
	}
}

// validateAndResolveEvent validates an event's fields and computes its absolute
// instants (StartAtUnix/PostAtUnix) from the wall-clock input + IANA timezone.
// On failure it writes the error response and returns false.
func (s *Server) validateAndResolveEvent(w http.ResponseWriter, ev *model.BookClubEvent) bool {
	ev.Title = strings.TrimSpace(ev.Title)
	if ev.Title == "" {
		writeError(w, http.StatusBadRequest, "Event title is required")
		return false
	}
	if ev.LengthHours < 1 || ev.LengthHours > 5 {
		writeError(w, http.StatusBadRequest, "Meeting length must be 1–5 hours")
		return false
	}
	tz := strings.TrimSpace(ev.Timezone)
	loc, err := time.LoadLocation(tz)
	if tz == "" || err != nil {
		writeError(w, http.StatusBadRequest, "Invalid timezone")
		return false
	}
	startAt, err := parseLocalInZone(ev.StartLocal, loc)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid start date/time")
		return false
	}
	postAt, err := parseLocalInZone(ev.PostAtLocal, loc)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid post date/time")
		return false
	}
	ev.Timezone = tz
	ev.Location = strings.TrimSpace(ev.Location)
	ev.Details = strings.TrimSpace(ev.Details)
	ev.Image = strings.TrimSpace(ev.Image)
	// Persist the absolute instants as UTC RFC-3339 strings (human-readable).
	ev.StartAt = startAt.UTC().Format(time.RFC3339)
	ev.PostAt = postAt.UTC().Format(time.RFC3339)
	return true
}

// parseLocalInZone parses a datetime-local wall-clock string in loc and returns
// the absolute instant as a time.Time.
func parseLocalInZone(value string, loc *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty datetime")
	}
	for _, layout := range eventTimeLayouts {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid datetime %q", value)
}

// ── Event images (upload + pick existing) ───────────────────────────────────

// eventImageDir is the upload directory for event images under the web root.
func (s *Server) eventImageDir() string {
	return filepath.Join(s.webRoot, filepath.FromSlash(eventImageRelDir))
}

// handleBookClubEventUpload stores a multipart event image under
// <webRoot>/images/bookclub/events and returns its full URL.
//
//	Endpoint:  POST /api/bookclub/events/upload
//	Auth:      admin, or any book-club page permission
//	Response:  {"url": "https://host/images/bookclub/events/event_....ext"}
func (s *Server) handleBookClubEventUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireAnyBookClub(w, r) {
		return
	}
	s.saveSingleImageUpload(w, r, eventImageRelDir, "event")
}

// handleBookClubEventImages lists existing event images (newest first) as full
// URLs so the admin can reuse one instead of uploading a duplicate.
//
//	Endpoint:  GET /api/bookclub/events/images
//	Auth:      admin, or any book-club page permission
//	Response:  {"images": ["https://host/images/bookclub/events/....png", ...]}
func (s *Server) handleBookClubEventImages(w http.ResponseWriter, r *http.Request) {
	if !s.requireAnyBookClub(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"images": s.listUploadedImageURLs(r, eventImageRelDir)})
}

// removeUploadedEventImageIfUnused deletes an uploaded event image, but only
// when it lives in the events upload directory and no other event still
// references it (event images are intentionally shareable/reusable).
func (s *Server) removeUploadedEventImageIfUnused(imageURL string, excludeEventID int64) {
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" || !strings.Contains(imageURL, "/images/bookclub/events/") {
		return
	}
	// Don't delete if any other event references the same image.
	for _, club := range bookClubs {
		events, err := s.store.ListBookClubEvents(club.Slug)
		if err != nil {
			return // be conservative on error: keep the file
		}
		for _, ev := range events {
			if ev.ID != excludeEventID && strings.TrimSpace(ev.Image) == imageURL {
				return
			}
		}
	}
	name := imageURL[strings.LastIndex(imageURL, "/")+1:]
	if name == "" || strings.ContainsAny(name, `/\`) {
		return
	}
	uploadDir, err := filepath.Abs(s.eventImageDir())
	if err != nil {
		return
	}
	target, err := filepath.Abs(filepath.Join(s.eventImageDir(), filepath.Base(name)))
	if err != nil || !strings.HasPrefix(target, uploadDir+string(os.PathSeparator)) {
		return
	}
	_ = os.Remove(target)
}

// isAllowedImageExt reports whether ext (lowercase, with dot) is a permitted
// uploaded-image extension.
func isAllowedImageExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	}
	return false
}

// ── Embed ───────────────────────────────────────────────────────────────────

// buildEventEmbed renders an event as a Discord embed. Times are emitted as
// Discord <t:…> timestamps so each viewer sees them in their own local zone.
// The date, location, and optional markdown details all render full-width in
// the description; the associated image is shown full-width at the bottom.
func buildEventEmbed(ev model.BookClubEvent) discordEmbed {
	var b strings.Builder
	// Convert the stored UTC RFC-3339 start instant to unix seconds for Discord's
	// <t:…> timestamp tokens (which each viewer sees rendered in their own zone).
	if startT, err := time.Parse(time.RFC3339, ev.StartAt); err == nil {
		startUnix := startT.Unix()
		end := startUnix + int64(ev.LengthHours)*3600
		// <t:…:F> = long date/time (includes the weekday), <t:…:t> = short time,
		// <t:…:R> = relative. Each renders in the viewer's own timezone. The end
		// time is appended after the full start date on the same row.
		fmt.Fprintf(&b, "🗓️ <t:%d:F> – <t:%d:t>\n", startUnix, end)
		fmt.Fprintf(&b, "⏳ <t:%d:R>", startUnix)
	}
	if loc := strings.TrimSpace(ev.Location); loc != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "📍 %s", loc)
	}
	if details := strings.TrimSpace(ev.Details); details != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(details)
	}
	return newEmbed().
		title(ev.Title).
		description(b.String()).
		image(ev.Image).
		footer("These dates are in your local time zone.").
		build()
}

// ── Scheduler ───────────────────────────────────────────────────────────────

// RunEventScheduler posts due, unposted book-club events to their club's events
// webhook on a fixed interval until ctx is cancelled. Safe to call in a
// goroutine; it returns when ctx is done.
func (s *Server) RunEventScheduler(ctx context.Context) {
	runScheduler(ctx, eventSchedulerInterval, s.postDueEvents)
}

// postDueEvents posts every event whose scheduled time has arrived. An event
// with no configured webhook is left pending (it posts once one is set); a
// failed post is retried on the next tick.
func (s *Server) postDueEvents() {
	events, err := s.store.DueBookClubEvents(time.Now())
	if err != nil {
		slog.Error("event scheduler: load due events", "error", err)
		return
	}
	for _, ev := range events {
		webhook, _ := s.store.GetSetting(eventsWebhookSettingKey(ev.ClubSlug))
		webhook = strings.TrimSpace(webhook)
		if webhook == "" {
			continue // no events webhook yet; try again next tick
		}
		if err := postDiscordEmbed(webhook, buildEventEmbed(ev)); err != nil {
			slog.Error("event scheduler: post event", "id", ev.ID, "error", err)
			continue // leave unposted; retry next tick
		}
		if err := s.store.MarkBookClubEventPosted(ev.ID); err != nil {
			slog.Error("event scheduler: mark posted", "id", ev.ID, "error", err)
		} else {
			slog.Info("posted scheduled book club event", "id", ev.ID, "club", ev.ClubSlug, "title", ev.Title)
		}
	}
}
