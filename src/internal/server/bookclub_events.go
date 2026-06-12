package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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
//	Auth:      admin
//	Response:  {"events": [...]}
func (s *Server) handleBookClubEventsList(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	club := strings.TrimSpace(r.URL.Query().Get("club"))
	if club == "" {
		club = defaultClubSlug
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
//	Auth:      admin
//	Request:   {"action": "create"|"update"|"delete"|"post_now", ...}
func (s *Server) handleBookClubEventsAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	req, err := readJSON[bookClubEventRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
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
	startUnix, err := parseLocalInZone(ev.StartLocal, loc)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid start date/time")
		return false
	}
	postUnix, err := parseLocalInZone(ev.PostAtLocal, loc)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid post date/time")
		return false
	}
	ev.Timezone = tz
	ev.Location = strings.TrimSpace(ev.Location)
	ev.Details = strings.TrimSpace(ev.Details)
	ev.Image = strings.TrimSpace(ev.Image)
	ev.StartAtUnix = startUnix
	ev.PostAtUnix = postUnix
	return true
}

// parseLocalInZone parses a datetime-local wall-clock string in loc and returns
// the absolute UTC unix seconds.
func parseLocalInZone(value string, loc *time.Location) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty datetime")
	}
	for _, layout := range eventTimeLayouts {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t.Unix(), nil
		}
	}
	return 0, fmt.Errorf("invalid datetime %q", value)
}

// ── Event images (upload + pick existing) ───────────────────────────────────

// eventImageDir is the upload directory for event images under the web root.
func (s *Server) eventImageDir() string {
	return filepath.Join(s.webRoot, "images", "bookclub", "events")
}

// handleBookClubEventUpload stores a multipart event image under
// <webRoot>/images/bookclub/events and returns its full URL.
//
//	Endpoint:  POST /api/bookclub/events/upload
//	Auth:      admin
//	Response:  {"url": "https://host/images/bookclub/events/event_....ext"}
func (s *Server) handleBookClubEventUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20) // 5 MB

	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Image upload failed (max 5MB)")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !isAllowedImageExt(ext) {
		writeError(w, http.StatusBadRequest, "Only jpg, png, webp, and gif images are allowed")
		return
	}

	if err := os.MkdirAll(s.eventImageDir(), 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create upload directory")
		return
	}
	filename := fmt.Sprintf("event_%d%s", time.Now().UnixNano(), ext)
	destPath := filepath.Join(s.eventImageDir(), filename)

	dst, err := os.Create(destPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save image")
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save image")
		return
	}

	fullURL := s.siteBaseURL(r) + "/images/bookclub/events/" + filename
	writeJSON(w, http.StatusOK, map[string]any{"url": fullURL})
}

// handleBookClubEventImages lists existing event images (newest first) as full
// URLs so the admin can reuse one instead of uploading a duplicate.
//
//	Endpoint:  GET /api/bookclub/events/images
//	Auth:      admin
//	Response:  {"images": ["https://host/images/bookclub/events/....png", ...]}
func (s *Server) handleBookClubEventImages(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	entries, err := os.ReadDir(s.eventImageDir())
	if err != nil {
		// Directory may not exist until the first upload — that's fine.
		writeJSON(w, http.StatusOK, map[string]any{"images": []string{}})
		return
	}

	type imgInfo struct {
		name string
		mod  time.Time
	}
	infos := make([]imgInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !isAllowedImageExt(strings.ToLower(filepath.Ext(e.Name()))) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		infos = append(infos, imgInfo{name: e.Name(), mod: fi.ModTime()})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].mod.After(infos[j].mod) })

	base := s.siteBaseURL(r) + "/images/bookclub/events/"
	images := make([]string, 0, len(infos))
	for _, info := range infos {
		images = append(images, base+info.name)
	}
	writeJSON(w, http.StatusOK, map[string]any{"images": images})
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
	embed := discordEmbed{
		Title: truncateRunes(ev.Title, 256),
		Color: 0xE53170, // accent pink
	}
	var b strings.Builder
	if ev.StartAtUnix > 0 {
		end := ev.StartAtUnix + int64(ev.LengthHours)*3600
		// <t:…:F> = long date/time (includes the weekday), <t:…:t> = short time,
		// <t:…:R> = relative. Each renders in the viewer's own timezone. The end
		// time is appended after the full start date on the same row.
		fmt.Fprintf(&b, "🗓️ <t:%d:F> – <t:%d:t>\n", ev.StartAtUnix, end)
		fmt.Fprintf(&b, "⏳ <t:%d:R>", ev.StartAtUnix)
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
	embed.Description = truncateRunes(b.String(), 4096)
	if isHTTPURL(ev.Image) {
		embed.Image = &discordEmbedImage{URL: ev.Image}
	}
	embed.Footer = &discordEmbedFooter{Text: "These dates are in your local time zone."}
	return embed
}

// ── Scheduler ───────────────────────────────────────────────────────────────

// RunEventScheduler posts due, unposted book-club events to their club's events
// webhook on a fixed interval until ctx is cancelled. Safe to call in a
// goroutine; it returns when ctx is done.
func (s *Server) RunEventScheduler(ctx context.Context) {
	ticker := time.NewTicker(eventSchedulerInterval)
	defer ticker.Stop()
	s.postDueEvents() // sweep immediately on startup (catch up after downtime)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.postDueEvents()
		}
	}
}

// postDueEvents posts every event whose scheduled time has arrived. An event
// with no configured webhook is left pending (it posts once one is set); a
// failed post is retried on the next tick.
func (s *Server) postDueEvents() {
	events, err := s.store.DueBookClubEvents(time.Now().Unix())
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
