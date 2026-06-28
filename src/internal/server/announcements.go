package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"regexp"
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
//	Auth:      admin, or a user granted this page's permission
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
//	Auth:      admin, or a user granted this page's permission
//	Request:   {"action": "create"|"update"|"delete", ...}
func (s *Server) handleAnnouncementTypesAction(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAnnounce) {
		return
	}
	req, err := readJSON[announcementTypeRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// A provided webhook URL must be a Discord webhook so the server can't be
	// pointed at an arbitrary outbound host. Empty = this type has no webhook.
	webhook := strings.TrimSpace(req.WebhookURL)
	if webhook != "" && !isDiscordWebhookURL(webhook) {
		writeError(w, http.StatusBadRequest, "Discord webhook URLs must look like https://discord.com/api/webhooks/…")
		return
	}

	switch req.Action {
	case "create":
		name := strings.TrimSpace(req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "Type name is required")
			return
		}
		id, err := s.store.CreateAnnouncementType(name, webhook)
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
		if err := s.store.UpdateAnnouncementType(req.ID, name, webhook); err != nil {
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

// ── Announcement roles (taggable Discord roles) ─────────────────────────────

// handleAnnouncementRolesList returns all taggable roles.
//
//	Endpoint:  GET /api/announcement-roles
//	Auth:      admin, or a user granted this page's permission
//	Response:  {"roles": [...]}
func (s *Server) handleAnnouncementRolesList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAnnounce) {
		return
	}
	roles, err := s.store.ListAnnouncementRoles()
	if err != nil {
		writeInternalError(w, "list announcement roles", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
}

// announcementRoleRequest is the JSON body for POST /api/announcement-roles.
type announcementRoleRequest struct {
	Action string `json:"action"`
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	RoleID string `json:"role_id"`
}

// handleAnnouncementRolesAction creates, updates, or deletes a taggable role.
//
//	Endpoint:  POST /api/announcement-roles
//	Auth:      admin, or a user granted this page's permission
//	Request:   {"action": "create"|"update"|"delete", ...}
func (s *Server) handleAnnouncementRolesAction(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAnnounce) {
		return
	}
	req, err := readJSON[announcementRoleRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Action {
	case "create":
		name := strings.TrimSpace(req.Name)
		roleID := strings.TrimSpace(req.RoleID)
		if name == "" {
			writeError(w, http.StatusBadRequest, "Role name is required")
			return
		}
		if !isDiscordSnowflake(roleID) {
			writeError(w, http.StatusBadRequest, "Role ID must be a Discord role ID (digits only)")
			return
		}
		id, err := s.store.CreateAnnouncementRole(name, roleID)
		if err != nil {
			writeInternalError(w, "create announcement role", err)
			return
		}
		saved, _ := s.store.GetAnnouncementRole(id)
		writeJSON(w, http.StatusCreated, map[string]any{"role": saved})

	case "update":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Role id is required")
			return
		}
		name := strings.TrimSpace(req.Name)
		roleID := strings.TrimSpace(req.RoleID)
		if name == "" {
			writeError(w, http.StatusBadRequest, "Role name is required")
			return
		}
		if !isDiscordSnowflake(roleID) {
			writeError(w, http.StatusBadRequest, "Role ID must be a Discord role ID (digits only)")
			return
		}
		if err := s.store.UpdateAnnouncementRole(req.ID, name, roleID); err != nil {
			writeInternalError(w, "update announcement role", err)
			return
		}
		saved, _ := s.store.GetAnnouncementRole(req.ID)
		writeJSON(w, http.StatusOK, map[string]any{"role": saved})

	case "delete":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Role id is required")
			return
		}
		count, err := s.store.CountAnnouncementsByRole(req.ID)
		if err != nil {
			writeInternalError(w, "count announcements by role", err)
			return
		}
		if count > 0 {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("This role is tagged by %d announcement(s). Change their tag first.", count))
			return
		}
		deleted, err := s.store.DeleteAnnouncementRole(req.ID)
		if err != nil {
			writeInternalError(w, "delete announcement role", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: create, update, delete")
	}
}

// isDiscordSnowflake reports whether s is a non-empty all-digit Discord ID.
func isDiscordSnowflake(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ── Announcements ───────────────────────────────────────────────────────────

// handleAnnouncementsList returns all announcements (with their type name).
//
//	Endpoint:  GET /api/announcements
//	Auth:      admin, or a user granted this page's permission
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
	OrderedIDs   []int64            `json:"ordered_ids"` // for "reorder"
}

// handleAnnouncementsAction creates, updates, deletes, sends, or skips an
// announcement.
//
//	Endpoint:  POST /api/announcements
//	Auth:      admin, or a user granted this page's permission
//	Request:   {"action": "create"|"update"|"delete"|"send_now"|"skip_next"|"reorder", ...}
func (s *Server) handleAnnouncementsAction(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseAnnounce) {
		return
	}
	req, err := readJSON[announcementRequest](w, r)
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
		if err := s.store.UpdateAnnouncement(&a); err != nil {
			writeInternalError(w, "update announcement", err)
			return
		}
		// Images are managed centrally on the System → Images page (a shared
		// library), so replacing an announcement's image no longer deletes the
		// old file here — it may be reused by another announcement.
		saved, _ := s.store.GetAnnouncement(a.ID)
		writeJSON(w, http.StatusOK, map[string]any{"announcement": saved})

	case "delete":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Announcement id is required")
			return
		}
		deleted, err := s.store.DeleteAnnouncement(req.ID)
		if err != nil {
			writeInternalError(w, "delete announcement", err)
			return
		}
		// Images are managed centrally on the System → Images page, so the
		// announcement's image/thumbnail files are left intact on delete.
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
		if err := postDiscordWebhook(typ.WebhookURL, s.buildAnnouncementMessage(*a)); err != nil {
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

	case "reorder":
		if err := s.store.BulkReorderAnnouncements(req.OrderedIDs); err != nil {
			writeInternalError(w, "reorder announcements", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: create, update, delete, send_now, skip_next, reorder")
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
	a.Thumbnail = strings.TrimSpace(a.Thumbnail)
	a.Color = strings.TrimSpace(a.Color)
	a.Location = strings.TrimSpace(a.Location)
	// Normalize the Discord timestamp styles to known letters (defaults otherwise).
	a.StartFormat = discordTimeStyle(a.StartFormat, defaultStartFormat)
	a.EndFormat = discordTimeStyle(a.EndFormat, defaultEndFormat)
	a.StartLocal = strings.TrimSpace(a.StartLocal)
	a.EndLocal = strings.TrimSpace(a.EndLocal)
	a.OnceLocal = strings.TrimSpace(a.OnceLocal)
	a.Buttons = sanitizeAnnouncementButtons(a.Buttons)

	// Optional role tag: "" (none), "everyone", or "role:<existing role id>".
	a.Mention = strings.TrimSpace(a.Mention)
	if a.Mention != "" && a.Mention != "everyone" {
		id, perr := strconv.ParseInt(strings.TrimPrefix(a.Mention, "role:"), 10, 64)
		if !strings.HasPrefix(a.Mention, "role:") || perr != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "Invalid role tag")
			return false
		}
		role, err := s.store.GetAnnouncementRole(id)
		if err != nil {
			writeInternalError(w, "load announcement role", err)
			return false
		}
		if role == nil {
			writeError(w, http.StatusBadRequest, "The selected role to tag no longer exists")
			return false
		}
	}

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

// localTimeLayouts are accepted wall-clock formats from an <input type=
// "datetime-local"> (seconds optional).
var localTimeLayouts = []string{"2006-01-02T15:04", "2006-01-02T15:04:05"}

// parseLocalInZone parses a datetime-local wall-clock string in loc and returns
// the absolute instant as a time.Time.
func parseLocalInZone(value string, loc *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty datetime")
	}
	for _, layout := range localTimeLayouts {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid datetime %q", value)
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
// announcement: an optional role tag in the message content (above the embed,
// where mentions actually notify), the embed itself, and an optional action row
// of link buttons.
func (s *Server) buildAnnouncementMessage(a model.Announcement) discordWebhookPayload {
	content, allowed := s.announcementMention(a)
	// For "dynamic dates", re-anchor the event start/end onto the day this post
	// goes out (now) so a recurring day-of announcement shows the current
	// occurrence. A no-op when the flag is off. Mutates the local copy only.
	a.StartAt, a.EndAt = dynamicEventTimes(a, time.Now())
	return discordWebhookPayload{
		Content:         content,
		Embeds:          []discordEmbed{buildAnnouncementEmbed(a)},
		Components:      announcementComponents(a),
		AllowedMentions: allowed,
	}
}

// dynamicEventTimes returns the start/end UTC instants the embed should display.
// When DynamicDates is on (and a start time is set), the stored StartLocal/EndLocal
// are treated as a template — their time-of-day, and how many days the end runs
// past the start — and re-anchored onto the day the announcement posts (`ref`, read
// in the announcement's timezone). Otherwise the stored StartAt/EndAt are returned
// unchanged. Any parse failure falls back to the stored values.
func dynamicEventTimes(a model.Announcement, ref time.Time) (startAt, endAt string) {
	if !a.DynamicDates || strings.TrimSpace(a.StartLocal) == "" {
		return a.StartAt, a.EndAt
	}
	loc := time.UTC
	if tz := strings.TrimSpace(a.Timezone); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	start, err := parseLocalInZone(a.StartLocal, loc)
	if err != nil {
		return a.StartAt, a.EndAt
	}
	day := ref.In(loc)
	newStart := time.Date(day.Year(), day.Month(), day.Day(), start.Hour(), start.Minute(), 0, 0, loc)
	startAt = newStart.UTC().Format(time.RFC3339)

	endAt = ""
	if strings.TrimSpace(a.EndLocal) != "" {
		if end, perr := parseLocalInZone(a.EndLocal, loc); perr == nil {
			// Keep the end's time-of-day, plus however many days it ran past the
			// start in the template (e.g. 10pm → 1am next day = +1 day).
			endDay := newStart.AddDate(0, 0, daysBetween(start, end))
			newEnd := time.Date(endDay.Year(), endDay.Month(), endDay.Day(), end.Hour(), end.Minute(), 0, 0, loc)
			endAt = newEnd.UTC().Format(time.RFC3339)
		}
	}
	return startAt, endAt
}

// daysBetween returns the whole-day difference between two wall-clock times'
// calendar dates (DST-safe via a noon anchor). Negative when b precedes a.
func daysBetween(a, b time.Time) int {
	da := time.Date(a.Year(), a.Month(), a.Day(), 12, 0, 0, 0, a.Location())
	db := time.Date(b.Year(), b.Month(), b.Day(), 12, 0, 0, 0, b.Location())
	return int(math.Round(db.Sub(da).Hours() / 24))
}

// announcementMention resolves an announcement's Mention selection into the
// message content string and the allowed-mentions whitelist that lets it ping:
//
//	""                  → no mention (nil whitelist)
//	"everyone"          → "@everyone", parse ["everyone"]
//	"role:<role_id>"    → "<@&DISCORD_ID>", roles [DISCORD_ID]
//
// A role whose managed entry was deleted (or has no Discord ID) resolves to no
// mention, so the announcement still posts — just without a tag.
func (s *Server) announcementMention(a model.Announcement) (string, *discordAllowedMentions) {
	mention := strings.TrimSpace(a.Mention)
	switch {
	case mention == "everyone":
		return "@everyone", &discordAllowedMentions{Parse: []string{"everyone"}}
	case strings.HasPrefix(mention, "role:"):
		id, err := strconv.ParseInt(strings.TrimPrefix(mention, "role:"), 10, 64)
		if err != nil || id <= 0 {
			return "", nil
		}
		role, err := s.store.GetAnnouncementRole(id)
		if err != nil || role == nil || strings.TrimSpace(role.RoleID) == "" {
			return "", nil
		}
		return fmt.Sprintf("<@&%s>", role.RoleID), &discordAllowedMentions{Parse: []string{}, Roles: []string{role.RoleID}}
	default:
		return "", nil
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
// (the author-chosen start/end styles, each independent) so each viewer sees
// their own zone. An optional location renders as an inline field beside it. The
// markdown details follow — normalized into Discord's markdown flavor first — as
// one or more full-width, headingless fields: details that would exceed Discord's
// 1024-char per-field cap are split across consecutive fields at a natural line
// break (see splitForEmbedFields) so nothing is truncated AND the time-first
// field order is preserved (a description would always render above the fields).
// An optional thumbnail renders small at the top-right and an optional image
// full-width at the bottom. The accent colour comes from the announcement (brand
// default if unset).
func buildAnnouncementEmbed(a model.Announcement) discordEmbed {
	b := newEmbed().
		title(a.Title).
		colorHex(a.Color)

	hasTime := false
	if startT, err := time.Parse(time.RFC3339, a.StartAt); err == nil {
		value := fmt.Sprintf("<t:%d:%s>", startT.Unix(), discordTimeStyle(a.StartFormat, defaultStartFormat))
		if endT, err := time.Parse(time.RFC3339, a.EndAt); err == nil {
			value += fmt.Sprintf(" to <t:%d:%s>", endT.Unix(), discordTimeStyle(a.EndFormat, defaultEndFormat))
		}
		b.field("🗓️ When", value, true)
		hasTime = true
	}
	// Optional location, inline so it sits beside the time when both are present.
	b.field("📍 Where", a.Location, true)
	// Details below the time, headingless (zero-width-space name) and full-width.
	// Split into multiple fields when over the 1024-char cap so nothing is cut.
	for _, chunk := range splitForEmbedFields(discordMarkdown(a.Details), embedFieldValueMax) {
		b.field(embedNoHeading, chunk, false)
	}
	// Optional small top-right thumbnail and large bottom image.
	b.thumbnail(a.Thumbnail)
	b.image(a.Image)
	if hasTime {
		b.footer("Times shown in your local time zone.")
	}
	return b.build()
}

// splitForEmbedFields splits text into chunks that each fit within limit runes
// (Discord's per-field value cap), so long announcement details render across
// consecutive fields instead of being truncated at the cap. Each chunk breaks at
// the last newline within the window (the natural marker), falling back to the
// last space, then to a hard cut when neither exists — so a split never lands
// mid-word when it can avoid it. Chunks are trimmed of surrounding whitespace
// (the boundary newline/space is consumed by the break); empty input yields nil
// and text already within the cap yields a single chunk.
//
// The frontend mirrors this in lib/announcementDetails.ts to warn the admin when
// their details will be split; keep the two in sync.
func splitForEmbedFields(text string, limit int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return []string{text}
	}

	var chunks []string
	for len(runes) > 0 {
		if len(runes) <= limit {
			if c := strings.TrimSpace(string(runes)); c != "" {
				chunks = append(chunks, c)
			}
			break
		}
		// Prefer the last newline in the window, else the last space; a break at
		// index 0 is unusable (no progress), so fall through to a hard cut.
		window := runes[:limit]
		cut := lastIndexRune(window, '\n')
		if cut <= 0 {
			cut = lastIndexRune(window, ' ')
		}
		if cut <= 0 {
			cut = limit
		}
		if c := strings.TrimSpace(string(runes[:cut])); c != "" {
			chunks = append(chunks, c)
		}
		runes = runes[cut:] // leading break rune is trimmed off the next chunk
	}
	return chunks
}

// lastIndexRune returns the index of the last occurrence of r in runes, or -1.
func lastIndexRune(runes []rune, r rune) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == r {
			return i
		}
	}
	return -1
}

// validTimeFormats is the set of Discord timestamp style letters an announcement
// may choose for its start/end display (t short time, T long time, d short date,
// D long date, f short date+time, F long date+time, R relative).
var validTimeFormats = map[string]bool{
	"t": true, "T": true, "d": true, "D": true, "f": true, "F": true, "R": true,
}

// defaultStartFormat / defaultEndFormat are the styles used when an announcement
// hasn't picked one: a full date+time for the start ("Saturday, June 13, 2026
// 7:00 PM") and just the time for the end ("9:00 PM"), since the end usually
// falls on the same day.
const (
	defaultStartFormat = "F"
	defaultEndFormat   = "t"
)

// discordTimeStyle returns the Discord <t:…:X> style letter for a chosen format,
// falling back to def for an empty or unrecognized value.
func discordTimeStyle(format, def string) string {
	format = strings.TrimSpace(format)
	if validTimeFormats[format] {
		return format
	}
	return def
}

// discordMarkdown normalizes the stored Milkdown (WYSIWYG) markdown into the
// flavor Discord renders, fixing artifacts the editor's serializer leaves behind
// that Discord doesn't understand:
//
//   - hard line breaks serialized as a literal <br>/<br/>/<br /> tag, which the
//     serializer follows with a source newline — Discord prints the tag as text,
//     so collapse the tag AND that trailing newline into a SINGLE newline. (The
//     earlier fix turned the tag into a newline but left the source newline,
//     producing a blank line between every line.)
//   - hard line breaks serialized as a trailing backslash ("…/\" then newline) —
//     Discord shows the literal "\", so drop the backslash and keep the break.
//     This is what makes a URL ending in "/" appear to gain a stray "\".
//   - "loose" lists, where the serializer separates each list item with a blank
//     line ("- a\n\n- b") — Discord renders that blank line between every bullet,
//     so collapse a blank line that sits between two list items into a single
//     newline (the blank line before/after the whole list, and real paragraphs,
//     are left untouched).
//   - Discord timestamp tokens (<t:1718…:F>) whose angle brackets the serializer
//     backslash-escapes ("\<t:…\>"), which stops Discord parsing them as a
//     timestamp — unescape the brackets so the timestamp renders.
func discordMarkdown(s string) string {
	if s == "" {
		return s
	}
	// Normalize line endings first: the editor stores CRLF, but every rule below
	// (and the blank-line collapsing) reasons in terms of "\n". Discord renders
	// "\n" fine, so emit LF throughout.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	// <br> hard break (+ its trailing source newline, if any) → one newline.
	s = brBreakRe.ReplaceAllString(s, "\n")
	// Backslash hard breaks: "\" at end of a line (or the string) → drop it.
	s = backslashBreakRe.ReplaceAllString(s, "$1")
	s = strings.TrimRight(s, "\\")
	// Never emit more than a paragraph break (guards against any stacked breaks).
	s = blankRe.ReplaceAllString(s, "\n\n")
	// Tighten loose lists: drop the blank line the serializer puts between items.
	s = tightenLists(s)
	// Unescape backslash-escaped angle brackets around Discord timestamp tokens.
	s = timestampEscapeRe.ReplaceAllString(s, "<t:$1$2>")
	return s
}

// tightenLists removes the blank line the Milkdown serializer inserts between the
// items of a "loose" list, so Discord renders the list without a gap between each
// bullet. A blank line is dropped only when the text before it is already inside a
// list (an item-marker line or its indented continuation) AND the next non-blank
// line is another item marker; the blank line before/after a whole list and the
// blank line between real paragraphs are preserved. Working line-by-line (rather
// than via one regex) is what lets it handle items that span multiple lines.
func tightenLists(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	inList := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			// Find the next non-blank line to see if the list continues.
			next := ""
			for j := i + 1; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) != "" {
					next = lines[j]
					break
				}
			}
			if inList && listItemRe.MatchString(next) {
				continue // drop the blank line between two list items
			}
			inList = false // a kept blank line ends the current list/paragraph
			out = append(out, line)
			continue
		}
		switch {
		case listItemRe.MatchString(line):
			inList = true
		case inList && (line[0] == ' ' || line[0] == '\t'):
			// indented continuation of the current item — stay in the list
		default:
			inList = false
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

var (
	// brBreakRe matches a <br> hard-break tag plus any inline spaces and a single
	// trailing newline, so the whole thing collapses to one line break (not a
	// blank line). Distinct from the shared brTagRe, which stripHTML relies on.
	brBreakRe = regexp.MustCompile(`(?i)<br\s*/?>[ \t]*\r?\n?`)
	// backslashBreakRe matches a backslash used as a markdown hard break: a "\"
	// immediately before a (CR)LF. The captured newline is preserved.
	backslashBreakRe = regexp.MustCompile(`\\(\r?\n)`)
	// listItemRe matches a bullet (-, *, +) or ordered (1. / 1)) list item line.
	// The required "[ \t]" after the marker keeps inline "*emphasis*" or a bare
	// "-" from being mistaken for a list item.
	listItemRe = regexp.MustCompile(`^[ \t]*(?:[-*+]|\d+[.)])[ \t]`)
	// timestampEscapeRe matches a Discord timestamp token whose "<"/">" may have
	// been backslash-escaped by the markdown serializer (e.g. "\<t:123:F\>").
	timestampEscapeRe = regexp.MustCompile(`\\?<t:(\d+)(:[tTdDfFR])?\\?>`)
)

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

// Announcement images are uploaded and managed centrally on the System → Images
// page (categories "Announcement Main" → images/announcements_main and
// "Announcement Thumbnail" → images/announcements_thumb). The editor's pickers
// read those categories via GET /api/images; there is no per-announcement upload
// or cleanup here anymore.

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
		err := postDiscordWebhook(typ.WebhookURL, s.buildAnnouncementMessage(a))
		if err != nil && !errors.Is(err, errWebhookAmbiguous) {
			// Definitely not delivered (HTTP error status, incl. 429 rate limit):
			// leave the cursor where it is so the next tick retries.
			slog.Error("announcement scheduler: post", "id", a.ID, "error", err)
			continue
		}
		// Success OR an ambiguous transport failure: advance the cursor either way.
		// On ambiguity the message may already be on Discord, so retrying would
		// duplicate it — we advance instead and log a warning. Recurring posts
		// resume at their next occurrence; a one-time post that truly failed needs
		// a manual resend.
		next, active := s.advanceCursor(a)
		if mErr := s.store.MarkAnnouncementPosted(a.ID, next, active); mErr != nil {
			slog.Error("announcement scheduler: mark posted", "id", a.ID, "error", mErr)
		} else if err != nil {
			slog.Warn("announcement post ambiguous; advanced cursor to avoid a duplicate",
				"id", a.ID, "title", a.Title, "error", err)
		} else {
			slog.Info("posted scheduled announcement", "id", a.ID, "title", a.Title)
		}
	}
}

// advanceCursor computes the next schedule cursor for an announcement after its
// current next_post_at fires. See advanceCursorAt; this binds it to the wall clock.
func (s *Server) advanceCursor(a model.Announcement) (nextPostAt string, active bool) {
	return s.advanceCursorAt(a, time.Now())
}

// advanceCursorAt computes the next schedule cursor as of `now`. Recurring kinds
// roll forward to the next occurrence STRICTLY IN THE FUTURE; a one-time schedule
// has no further occurrence (returns "", false → deactivated).
//
// The anchor is the later of `now` and the just-fired cursor. Anchoring on `now`
// (not the stale cursor) is essential: when an announcement is overdue by more
// than one period — e.g. the server was down across a scheduled slot — advancing
// from the old cursor would land on ANOTHER past slot, leaving it still due so it
// re-posts every tick until it catches up (the double-post bug). Anchoring on now
// jumps straight to the next future slot; missed occurrences are skipped, not
// replayed.
func (s *Server) advanceCursorAt(a model.Announcement, now time.Time) (nextPostAt string, active bool) {
	if a.ScheduleKind == "" || a.ScheduleKind == "once" {
		return "", false
	}
	after := now
	if t, err := time.Parse(time.RFC3339, a.NextPostAt); err == nil && t.After(after) {
		after = t
	}
	next := nextAnnouncementOccurrence(a, after)
	return next, next != ""
}
