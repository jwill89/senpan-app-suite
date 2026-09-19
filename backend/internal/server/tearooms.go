package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"app-suite/internal/model"
)

// -- Tea Rooms (Senpan Tea House -> Tea Rooms) --------------------------------
//
// A tea room is a single-table entity (see model.TeaRoom): admins manage a
// drag-orderable list of bookable rooms and post each as a Discord embed to one
// shared webhook. The list is also exposed read-only through a public,
// cross-origin API so an external Carrd site can render live availability/pricing.
//
// Admin CRUD + the toggle/post/reorder commands are gated by permTeahouseTeaRooms;
// the /public endpoints are unauthenticated and send `Access-Control-Allow-Origin:
// *` so any site can fetch them.

// teaRoomWebhookSettingKey is the settings-table key holding the single shared
// Discord webhook that Tea Rooms post to. It is deliberately NOT in settingsKeys
// (server/settings.go), so it never leaks through the public GET /api/settings -
// it's read/written only through the permission-gated tea-room endpoints.
const teaRoomWebhookSettingKey = "tearoom_webhook_url"

// maxTeaRoomHashtags caps how many hashtags a room keeps (abuse guard).
const maxTeaRoomHashtags = 30

// -- Admin list + CRUD --------------------------------------------------------

// handleTeaRoomsList returns every tea room (admin order) plus the shared Discord
// webhook (safe here - the endpoint is permission-gated, unlike public settings).
//
//	Endpoint:  GET /api/tea-rooms
//	Auth:      admin, or a user granted teahouse-tea-rooms
//	Response:  {"tea_rooms": [...], "webhook_url": "..."}
func (s *Server) handleTeaRoomsList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseTeaRooms) {
		return
	}
	rooms, err := s.store.ListTeaRooms()
	if err != nil {
		writeInternalError(w, "list tea rooms", err)
		return
	}
	webhook, _ := s.store.GetSetting(teaRoomWebhookSettingKey)
	writeJSON(w, http.StatusOK, model.TeaRoomsResponse{TeaRooms: rooms, WebhookURL: webhook})
}

// teaRoomWriteRequest is the JSON body for creating (POST /api/tea-rooms) or
// replacing (PUT /api/tea-rooms/{id}) a tea room. The id comes from the path on PUT.
type teaRoomWriteRequest struct {
	TeaRoom model.TeaRoom `json:"tea_room"`
}

// validateAndSanitize normalizes the room's fields in place and reports the first
// validation error (writing the 400 response). Returns true when the room is valid.
func (req *teaRoomWriteRequest) validateAndSanitize(w http.ResponseWriter) bool {
	t := &req.TeaRoom
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		writeError(w, http.StatusBadRequest, "Room name is required")
		return false
	}
	t.RoomNumber = strings.TrimSpace(t.RoomNumber)
	if t.RoomNumber == "" {
		writeError(w, http.StatusBadRequest, "Room number is required")
		return false
	}
	if t.CostPerHalfHour < 0 {
		writeError(w, http.StatusBadRequest, "Cost cannot be negative")
		return false
	}
	t.Subtitle = strings.TrimSpace(t.Subtitle)
	t.RoomOwner = strings.TrimSpace(t.RoomOwner)
	t.Description = strings.TrimSpace(t.Description)
	t.Image = strings.TrimSpace(t.Image)
	t.Color = strings.TrimSpace(t.Color)
	t.Hashtags = normalizeHashtags(t.Hashtags)
	until, ok := normalizeLockUntil(w, t.Locked, t.LockedUntil)
	if !ok {
		return false
	}
	t.LockedUntil = until
	return true
}

// -- Room locks ---------------------------------------------------------------
//
// A room can be locked (booked out) either indefinitely - lifted when an admin
// unlocks it, which is how locking has always worked - or until a set moment,
// after which the background sweeper unlocks it and tells every admin client
// (including the in-game plugin, which alerts its operator). `locked_until` is
// therefore only ever read while `locked` is set, and unlocking clears it.

// parseTeaRoomLockTime parses a lock expiry into a UTC instant. Values are stored
// canonically (every write goes through normalizeLockUntil), so RFC-3339 is the
// only accepted form; anything else is a client sending something we never wrote.
// Returns the instant and whether it parsed.
func parseTeaRoomLockTime(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// normalizeLockUntil canonicalizes a lock expiry for storage and reports whether
// it was acceptable, writing the 400 itself when it wasn't. An unlocked room has
// no expiry (so a stale time can't outlive the lock it belonged to), and a locked
// room may have none - that is the manual-unlock lock. A time already in the past
// is accepted rather than refused: it simply means the lock is over, and the next
// sweep lifts it. Clients guard against typing one, but a form submitted a moment
// after its own expiry passed must not fail on an unrelated edit.
func normalizeLockUntil(w http.ResponseWriter, locked bool, lockedUntil string) (string, bool) {
	if !locked {
		return "", true
	}
	s := strings.TrimSpace(lockedUntil)
	if s == "" {
		return "", true
	}
	at, ok := parseTeaRoomLockTime(s)
	if !ok {
		writeError(w, http.StatusBadRequest,
			"The unlock time must be a date and time (RFC-3339, e.g. 2026-06-13T20:00:00Z).")
		return "", false
	}
	return at.Format(time.RFC3339), true
}

// checkTeaRoomNumberUnique reports whether room_number is free to use (unique, or
// already this room's). It writes a 400 and returns false when another room owns
// it - a friendly guard ahead of the DB's UNIQUE index backstop.
func (s *Server) checkTeaRoomNumberUnique(w http.ResponseWriter, number string, exceptID int64) bool {
	other, err := s.store.GetTeaRoomByNumber(number)
	if err != nil {
		writeInternalError(w, "check tea room number", err)
		return false
	}
	if other != nil && other.ID != exceptID {
		writeError(w, http.StatusBadRequest, "That room number is already in use by another room.")
		return false
	}
	return true
}

// handleTeaRoomCreate creates a tea room.
//
//	Endpoint:  POST /api/tea-rooms
//	Auth:      admin, or a user granted teahouse-tea-rooms
//	Response:  201 {"tea_room": TeaRoom}
func (s *Server) handleTeaRoomCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseTeaRooms) {
		return
	}
	req, err := readJSON[teaRoomWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if !req.validateAndSanitize(w) {
		return
	}
	if !s.checkTeaRoomNumberUnique(w, req.TeaRoom.RoomNumber, 0) {
		return
	}
	id, err := s.store.CreateTeaRoom(&req.TeaRoom)
	if err != nil {
		writeInternalError(w, "create tea room", err)
		return
	}
	saved, _ := s.store.GetTeaRoom(id)
	writeJSON(w, http.StatusCreated, model.TeaRoomResponse{TeaRoom: saved})
}

// handleTeaRoomUpdate replaces a tea room's editable fields (its sort_order is
// preserved - reordering is a separate bulk operation).
//
//	Endpoint:  PUT /api/tea-rooms/{id}
//	Auth:      admin, or a user granted teahouse-tea-rooms
//	Response:  200 {"tea_room": TeaRoom}
func (s *Server) handleTeaRoomUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseTeaRooms) {
		return
	}
	id, ok := pathInt64(w, r, "id", "tea room")
	if !ok {
		return
	}
	existing, err := s.store.GetTeaRoom(id)
	if err != nil {
		writeInternalError(w, "load tea room for update", err)
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "Tea room not found")
		return
	}
	req, err := readJSON[teaRoomWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if !req.validateAndSanitize(w) {
		return
	}
	if !s.checkTeaRoomNumberUnique(w, req.TeaRoom.RoomNumber, id) {
		return
	}
	req.TeaRoom.ID = id
	if err := s.store.UpdateTeaRoom(&req.TeaRoom); err != nil {
		writeInternalError(w, "update tea room", err)
		return
	}
	saved, _ := s.store.GetTeaRoom(id)
	writeJSON(w, http.StatusOK, model.TeaRoomResponse{TeaRoom: saved})
}

// teaRoomPatchRequest is the JSON body for PATCH /api/tea-rooms/{id}: a partial
// update of the quick-toggle flags. Absent (nil) fields are left unchanged, so the
// same endpoint backs the open/closed and discounted toggles and the lock.
//
// `locked` and `locked_until` are separate fields rather than one, so a lock can
// be placed and re-timed independently: sending `locked` alone locks (or unlocks)
// the room, `locked_until` alone re-times a lock already in place, and the two
// together do both. A lock with no `locked_until` stands until someone unlocks it.
type teaRoomPatchRequest struct {
	Open        *bool   `json:"open"`
	Discounted  *bool   `json:"discounted"`
	Locked      *bool   `json:"locked"`
	LockedUntil *string `json:"locked_until"`
}

// handleTeaRoomPatch toggles a room's open and/or discounted flag, and sets or
// lifts its lock.
//
//	Endpoint:  PATCH /api/tea-rooms/{id}
//	Auth:      admin, or a user granted teahouse-tea-rooms
//	Response:  200 {"tea_room": TeaRoom}
func (s *Server) handleTeaRoomPatch(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseTeaRooms) {
		return
	}
	id, ok := pathInt64(w, r, "id", "tea room")
	if !ok {
		return
	}
	existing, err := s.store.GetTeaRoom(id)
	if err != nil {
		writeInternalError(w, "load tea room for patch", err)
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "Tea room not found")
		return
	}
	req, err := readJSON[teaRoomPatchRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	// Work out the lock BEFORE writing anything: the flags below each go to the
	// database on their own, so a request that toggles a flag and carries an
	// unreadable unlock time would otherwise 400 with the toggle already applied.
	//
	// Start from what the room already is, so either lock field can be sent alone.
	// An unlocked room never carries an expiry (SetTeaRoomLock and the sweeper both
	// clear it), so locking without a time always starts from "no expiry".
	locked, lockedUntil := existing.Locked, existing.LockedUntil
	if req.Locked != nil {
		locked = *req.Locked
	}
	if req.LockedUntil != nil {
		lockedUntil = *req.LockedUntil
	}
	if lockedUntil, ok = normalizeLockUntil(w, locked, lockedUntil); !ok {
		return
	}

	if req.Open != nil {
		if err := s.store.SetTeaRoomOpen(id, *req.Open); err != nil {
			writeInternalError(w, "toggle tea room open", err)
			return
		}
	}
	if req.Discounted != nil {
		if err := s.store.SetTeaRoomDiscounted(id, *req.Discounted); err != nil {
			writeInternalError(w, "toggle tea room discounted", err)
			return
		}
	}
	if req.Locked != nil || req.LockedUntil != nil {
		if err := s.store.SetTeaRoomLock(id, locked, lockedUntil); err != nil {
			writeInternalError(w, "set tea room lock", err)
			return
		}
	}
	saved, _ := s.store.GetTeaRoom(id)
	writeJSON(w, http.StatusOK, model.TeaRoomResponse{TeaRoom: saved})
}

// handleTeaRoomDelete deletes a tea room. Its image is a shared library asset
// (System -> Images), so the file is left intact.
//
//	Endpoint:  DELETE /api/tea-rooms/{id}
//	Auth:      admin, or a user granted teahouse-tea-rooms
//	Response:  204 No Content
func (s *Server) handleTeaRoomDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseTeaRooms) {
		return
	}
	id, ok := pathInt64(w, r, "id", "tea room")
	if !ok {
		return
	}
	if _, err := s.store.DeleteTeaRoom(id); err != nil {
		writeInternalError(w, "delete tea room", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// teaRoomReorderRequest is the JSON body for POST /api/tea-rooms/reorder.
type teaRoomReorderRequest struct {
	OrderedIDs []int64 `json:"ordered_ids"`
}

// handleTeaRoomsReorder persists a new drag-and-drop order (top-first ids).
//
//	Endpoint:  POST /api/tea-rooms/reorder
//	Auth:      admin, or a user granted teahouse-tea-rooms
//	Response:  200 {"ok": true}
func (s *Server) handleTeaRoomsReorder(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseTeaRooms) {
		return
	}
	req, err := readJSON[teaRoomReorderRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if err := s.store.BulkReorderTeaRooms(req.OrderedIDs); err != nil {
		writeInternalError(w, "reorder tea rooms", err)
		return
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}

// handleTeaRoomPost posts a tea room's embed to the shared Discord webhook now.
//
//	Endpoint:  POST /api/tea-rooms/{id}/post
//	Auth:      admin, or a user granted teahouse-tea-rooms
//	Response:  200 {"tea_room": TeaRoom}
func (s *Server) handleTeaRoomPost(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseTeaRooms) {
		return
	}
	id, ok := pathInt64(w, r, "id", "tea room")
	if !ok {
		return
	}
	room, err := s.store.GetTeaRoom(id)
	if err != nil {
		writeInternalError(w, "load tea room for post", err)
		return
	}
	if room == nil {
		writeError(w, http.StatusNotFound, "Tea room not found")
		return
	}
	webhook, _ := s.store.GetSetting(teaRoomWebhookSettingKey)
	if strings.TrimSpace(webhook) == "" {
		writeError(w, http.StatusBadRequest, "No Tea Rooms Discord webhook is configured. Set one on the Tea Rooms page first.")
		return
	}
	target := webhookTarget{Kind: "tea_room", Name: room.Name}
	if err := s.postDiscordEmbed(r.Context(), target, webhook, buildTeaRoomEmbed(*room)); err != nil {
		writeUpstreamError(w, fmt.Sprintf("post tea room %d", id), err)
		return
	}
	writeJSON(w, http.StatusOK, model.TeaRoomResponse{TeaRoom: room})
}

// -- Lock-expiry scheduler ----------------------------------------------------

// teaRoomLockSchedulerInterval is how often the sweeper looks for locks that have
// run out. Shorter than the announcement scheduler's 30s: an expiry is a moment
// staff are waiting on (the plugin alerts the operator in game the instant it
// lands), and the sweep is one small query against a table of a few dozen rows.
const teaRoomLockSchedulerInterval = 15 * time.Second

// RunTeaRoomLockScheduler lifts tea-room locks whose set expiry has passed, until
// ctx is cancelled. Safe to call in a goroutine.
func (s *Server) RunTeaRoomLockScheduler(ctx context.Context) {
	runScheduler(ctx, "tea-room-locks", teaRoomLockSchedulerInterval, s.expireDueTeaRoomLocks)
}

// expireDueTeaRoomLocks unlocks every room whose lock expiry has arrived and
// announces each one. Locks with no expiry are never touched - they are the ones
// waiting on a person. Like the announcement sweep, it runs on startup too, so a
// lock that ran out while the process was down is lifted as soon as it is back.
func (s *Server) expireDueTeaRoomLocks() {
	rooms, err := s.store.ExpiringTeaRoomLocks()
	if err != nil {
		slog.Error("tea room lock scheduler: load expiring locks", "error", err)
		return
	}
	now := time.Now()
	for _, room := range rooms {
		at, ok := parseTeaRoomLockTime(room.LockedUntil)
		// An expiry we can't read is not one we can honour, and leaving it would
		// wedge the room locked while logging the same complaint every sweep. Treat
		// it as due: the room unlocks, the warning is written once, and the bad
		// value is gone with it.
		if !ok {
			slog.Warn("tea room lock has an unreadable expiry; unlocking it",
				"id", room.ID, "name", room.Name, "locked_until", room.LockedUntil)
		} else if now.Before(at) {
			continue
		}
		applied, err := s.store.ExpireTeaRoomLock(room.ID, room.LockedUntil)
		if err != nil {
			slog.Error("tea room lock scheduler: expire lock", "id", room.ID, "error", err)
			continue
		}
		if !applied {
			// An admin re-locked or re-timed the room while this sweep was running;
			// their lock wins over one judged from a snapshot taken before it.
			slog.Info("tea room lock changed during the sweep; leaving the admin's lock alone",
				"id", room.ID, "name", room.Name)
			continue
		}
		slog.Info("tea room lock expired", "id", room.ID, "name", room.Name, "room_number", room.RoomNumber)
		s.broadcastTeaRoomUnlocked(room)
		// The rooms list itself changed, and nothing here went through the admin
		// mutation middleware (no request made this happen), so invalidate it too.
		s.broadcastResourceChanged("tea-rooms")
	}
}

// broadcastTeaRoomUnlocked tells admin clients that a room's lock has just run
// out. It carries the room's name and number rather than only its id because the
// consumers announce it rather than render it: the in-game plugin alerts its
// operator by name the moment this lands, and looking the name up would mean a
// REST round-trip for a room the server has right here. Admin channel only - the
// tea-room list is permission-gated, and no player client has any use for it.
func (s *Server) broadcastTeaRoomUnlocked(room model.TeaRoom) {
	s.hub.BroadcastToAdmins(struct {
		Type       string `json:"type"`
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		RoomNumber string `json:"room_number"`
	}{Type: "tea_room_unlocked", ID: room.ID, Name: room.Name, RoomNumber: room.RoomNumber})
}

// -- Shared Discord webhook --------------------------------------------------

// teaRoomWebhookRequest is the JSON body for PUT /api/tea-rooms/webhook.
type teaRoomWebhookRequest struct {
	WebhookURL string `json:"webhook_url"`
}

// handleTeaRoomWebhookSet stores the single shared Discord webhook that Tea Rooms
// post to. An empty value clears it.
//
//	Endpoint:  PUT /api/tea-rooms/webhook
//	Auth:      admin, or a user granted teahouse-tea-rooms
//	Response:  200 {"webhook_url": "..."}
func (s *Server) handleTeaRoomWebhookSet(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseTeaRooms) {
		return
	}
	req, err := readJSON[teaRoomWebhookRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	webhook := strings.TrimSpace(req.WebhookURL)
	// A provided webhook must be a Discord webhook so the server can't be pointed at
	// an arbitrary outbound host. Empty clears it.
	if webhook != "" && !isDiscordWebhookURL(webhook) {
		writeError(w, http.StatusBadRequest, "Discord webhook URLs must look like https://discord.com/api/webhooks/...")
		return
	}
	if err := s.store.SetSetting(teaRoomWebhookSettingKey, webhook); err != nil {
		writeInternalError(w, "save tea room webhook", err)
		return
	}
	writeJSON(w, http.StatusOK, model.TeaRoomWebhookResponse{WebhookURL: webhook})
}

// -- Public API (cross-origin, read-only) ------------------------------------

// handleTeaRoomsPublic returns every tea room for an external site (e.g. a Carrd
// embed). Unauthenticated and cross-origin (`Access-Control-Allow-Origin: *`); the
// data is non-sensitive room availability/pricing and carries no webhook.
//
//	Endpoint:  GET /api/tea-rooms/public
//	Auth:      public
//	Response:  {"tea_rooms": [...]}
func (s *Server) handleTeaRoomsPublic(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	rooms, err := s.store.ListTeaRooms()
	if err != nil {
		writeInternalError(w, "list tea rooms (public)", err)
		return
	}
	writeJSON(w, http.StatusOK, model.TeaRoomsPublicResponse{TeaRooms: rooms})
}

// handleTeaRoomPublic returns a single tea room with all its data + status flags,
// looked up by its ROOM NUMBER (the room's public key, so an external site keys
// off the one number the admin already knows).
//
//	Endpoint:  GET /api/tea-rooms/public/{number}
//	Auth:      public
//	Response:  {"tea_room": TeaRoom}
func (s *Server) handleTeaRoomPublic(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	number := strings.TrimSpace(r.PathValue("number"))
	if number == "" {
		writeError(w, http.StatusNotFound, "Tea room not found")
		return
	}
	room, err := s.store.GetTeaRoomByNumber(number)
	if err != nil {
		writeInternalError(w, "load tea room (public)", err)
		return
	}
	if room == nil {
		writeError(w, http.StatusNotFound, "Tea room not found")
		return
	}
	writeJSON(w, http.StatusOK, model.TeaRoomPublicResponse{TeaRoom: room})
}

// -- Embed --------------------------------------------------------------------

// buildTeaRoomEmbed renders a tea room as a Discord embed: the room name as the
// title, the markdown description as the body, then three inline fields - the
// per-half-hour cost (halved, with a note, when discounted), the room number, and
// the open/closed status. Hashtags render in the footer, always capitalized. The
// room image renders full-width at the bottom, and the accent colour comes from
// the room (brand default when unset).
func buildTeaRoomEmbed(t model.TeaRoom) discordEmbed {
	b := newEmbed().title(t.Name).colorHex(t.Color)

	// Body: the markdown description.
	b.description(discordMarkdown(strings.TrimSpace(t.Description)))

	// Cost - full price, or the fixed 50%-off price plus a note when discounted.
	// Inline so the room number + status sit beside it.
	if t.Discounted {
		b.field("💰 Cost", fmt.Sprintf("~~%s gil~~ **%s gil**/half hour\n**Currently Discounted!**",
			formatGil(t.CostPerHalfHour), formatGil(t.CostPerHalfHour/2)), true)
	} else {
		b.field("💰 Cost", fmt.Sprintf("%s gil/half hour", formatGil(t.CostPerHalfHour)), true)
	}

	// Room number + open/closed status, inline beside the cost.
	b.field("🔢 Room Number", t.RoomNumber, true)
	b.field("🚪 Status", boolLabel(t.Open, "Open", "Closed"), true)

	// Hashtags in the footer, always capitalized (e.g. "#Cozy #Private").
	if tags := strings.TrimSpace(t.Hashtags); tags != "" {
		b.footer(capitalizeHashtags(tags))
	}

	b.image(t.Image)
	return b.build()
}

// boolLabel returns yes when b is true, else no - a tiny helper for the embed's
// status fields (Seasonal/Permanent, Open/Closed).
func boolLabel(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

// formatGil formats a gil amount with thousands separators, e.g. 125000 ->
// "125,000". Negative values keep their sign (though costs are validated >= 0).
func formatGil(n int64) string {
	s := strconv.FormatInt(n, 10)
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(s[i])
	}
	return sign + b.String()
}

// normalizeHashtags turns free-form input ("cozy, private" / "#cozy #private")
// into a normalized, deduplicated, space-separated "#tag" list. Splitting on
// commas and whitespace, it strips a leading '#', drops blanks and case-insensitive
// duplicates, re-prefixes each with '#', and caps the count.
func normalizeHashtags(raw string) string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	seen := make(map[string]bool, len(fields))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		tag := strings.TrimSpace(strings.TrimLeft(f, "#"))
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, "#"+tag)
		if len(out) >= maxTeaRoomHashtags {
			break
		}
	}
	return strings.Join(out, " ")
}

// capitalizeHashtags upper-cases the first letter of each "#tag" in a normalized
// hashtag string (e.g. "#cozy #private" -> "#Cozy #Private"), for the embed footer.
// Only the leading letter is forced up; the rest of each tag is left as stored.
func capitalizeHashtags(tags string) string {
	fields := strings.Fields(tags)
	for i, f := range fields {
		tag := strings.TrimPrefix(f, "#")
		if tag == "" {
			continue
		}
		r := []rune(tag)
		r[0] = unicode.ToUpper(r[0])
		fields[i] = "#" + string(r)
	}
	return strings.Join(fields, " ")
}
