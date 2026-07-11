package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"app-suite/internal/model"
)

// ── Tea Rooms (Senpan Tea House → Tea Rooms) ────────────────────────────────
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
// (server/settings.go), so it never leaks through the public GET /api/settings —
// it's read/written only through the permission-gated tea-room endpoints.
const teaRoomWebhookSettingKey = "tearoom_webhook_url"

// maxTeaRoomHashtags caps how many hashtags a room keeps (abuse guard).
const maxTeaRoomHashtags = 30

// ── Admin list + CRUD ────────────────────────────────────────────────────────

// handleTeaRoomsList returns every tea room (admin order) plus the shared Discord
// webhook (safe here — the endpoint is permission-gated, unlike public settings).
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
	t.Description = strings.TrimSpace(t.Description)
	t.Image = strings.TrimSpace(t.Image)
	t.Color = strings.TrimSpace(t.Color)
	t.Hashtags = normalizeHashtags(t.Hashtags)
	return true
}

// checkTeaRoomNumberUnique reports whether room_number is free to use (unique, or
// already this room's). It writes a 400 and returns false when another room owns
// it — a friendly guard ahead of the DB's UNIQUE index backstop.
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
// preserved — reordering is a separate bulk operation).
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
// same endpoint backs both the open/closed and the discounted toggles.
type teaRoomPatchRequest struct {
	Open       *bool `json:"open"`
	Discounted *bool `json:"discounted"`
}

// handleTeaRoomPatch toggles a room's open and/or discounted flag.
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
	saved, _ := s.store.GetTeaRoom(id)
	writeJSON(w, http.StatusOK, model.TeaRoomResponse{TeaRoom: saved})
}

// handleTeaRoomDelete deletes a tea room. Its image is a shared library asset
// (System → Images), so the file is left intact.
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
	if err := postDiscordEmbed(webhook, buildTeaRoomEmbed(*room)); err != nil {
		writeError(w, http.StatusBadGateway, "Failed to post to Discord: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, model.TeaRoomResponse{TeaRoom: room})
}

// ── Shared Discord webhook ──────────────────────────────────────────────────

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
		writeError(w, http.StatusBadRequest, "Discord webhook URLs must look like https://discord.com/api/webhooks/…")
		return
	}
	if err := s.store.SetSetting(teaRoomWebhookSettingKey, webhook); err != nil {
		writeInternalError(w, "save tea room webhook", err)
		return
	}
	writeJSON(w, http.StatusOK, model.TeaRoomWebhookResponse{WebhookURL: webhook})
}

// ── Public API (cross-origin, read-only) ────────────────────────────────────

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

// ── Embed ────────────────────────────────────────────────────────────────────

// buildTeaRoomEmbed renders a tea room as a Discord embed: the room name as the
// title, the markdown description as the body, then three inline fields — the
// per-half-hour cost (halved, with a note, when discounted), the room number, and
// the open/closed status. Hashtags render in the footer, always capitalized. The
// room image renders full-width at the bottom, and the accent colour comes from
// the room (brand default when unset).
func buildTeaRoomEmbed(t model.TeaRoom) discordEmbed {
	b := newEmbed().title(t.Name).colorHex(t.Color)

	// Body: the markdown description.
	b.description(discordMarkdown(strings.TrimSpace(t.Description)))

	// Cost — full price, or the fixed 50%-off price plus a note when discounted.
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

// boolLabel returns yes when b is true, else no — a tiny helper for the embed's
// status fields (Seasonal/Permanent, Open/Closed).
func boolLabel(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

// formatGil formats a gil amount with thousands separators, e.g. 125000 →
// "125,000". Negative values keep their sign (though costs are validated ≥ 0).
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
// hashtag string (e.g. "#cozy #private" → "#Cozy #Private"), for the embed footer.
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
