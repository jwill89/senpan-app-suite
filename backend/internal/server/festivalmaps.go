package server

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// -- Festival Map (admin CRUD + the public interactive map) -------------------
//
// A Festival Map (see model.FestivalMap) is a floor plan: a base image with
// stalls drawn on top of it at %-based placements. The admin authors the map and
// publishes it; the public endpoints need no auth and show published maps only.
//
// As with stamp rallies, the store stays pure data access and everything that
// depends on the clock - whether the festival is running, whether a stall is open
// right now - is computed here against time.Now, reusing parseRaffleTime for the
// shared UTC parsing.

// hexColor matches the "#rrggbb" form the stall color picker produces. Anything
// else is dropped on save so a stall's color can only ever be a literal the
// stylesheet can use (or empty, meaning "the stall type's default").
var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// sanitizeColor keeps a "#rrggbb" color, lowercased, and discards anything else.
func sanitizeColor(color string) string {
	color = strings.TrimSpace(color)
	if hexColor.MatchString(color) {
		return strings.ToLower(color)
	}
	return ""
}

// mapSlug matches an accepted shortcode: lowercase alphanumeric words joined by
// single dashes, 2-64 characters. Deliberately narrow - the shortcode goes in a
// URL path where it shares a slot with the numeric id.
var mapSlug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// allDigits matches a shortcode made only of digits, which is refused: it would
// share the public path segment with a map id and there would be no way to tell
// which one a link meant.
var allDigits = regexp.MustCompile(`^[0-9]+$`)

// normalizeMapSlug settles whatever the admin typed into a storable shortcode:
// trimmed, lowercased, with runs of spaces and underscores folded into the single
// dash the pattern allows. That way "Obon 2026" and "obon_2026" both save as
// "obon-2026" rather than being bounced back as invalid.
func normalizeMapSlug(slug string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	slug = slugSeparators.ReplaceAllString(slug, "-")
	return strings.Trim(slug, "-")
}

// slugSeparators matches the runs of whitespace/underscore/dash that fold into
// one dash.
var slugSeparators = regexp.MustCompile(`[\s_-]+`)

// slugError returns the message rejecting a shortcode, or "" when it is usable.
// An empty shortcode is always fine - it simply means the map is reachable by id.
func slugError(slug string) string {
	if slug == "" {
		return ""
	}
	if allDigits.MatchString(slug) {
		return "A shortcode can't be only numbers - it would be indistinguishable from a map id."
	}
	if len(slug) < 2 || len(slug) > 64 || !mapSlug.MatchString(slug) {
		return "A shortcode may use letters, numbers and dashes only (2-64 characters), e.g. \"obon-2026\"."
	}
	return ""
}

// checkMapSlugUnique reports whether a shortcode is free (or is already this
// map's own). It writes a 409 and returns false when another map holds it - a
// friendly guard ahead of the partial UNIQUE index's backstop, mirroring the
// tea-room room-number check.
func (s *Server) checkMapSlugUnique(w http.ResponseWriter, slug string, exceptID int64) bool {
	if slug == "" {
		return true
	}
	other, err := s.store.GetFestivalMapBySlug(slug)
	if err != nil {
		writeInternalError(w, "check festival map slug", err)
		return false
	}
	if other != nil && other.ID != exceptID {
		writeError(w, http.StatusConflict, "That shortcode is already used by another festival map.")
		return false
	}
	return true
}

// sanitizeEventTimes drops entries with no start (an unstarted range says
// nothing) and normalizes the rest, so a stored list is always usable. Returns a
// non-nil slice so the JSON column holds "[]" rather than "null".
func sanitizeEventTimes(times []model.EventTime) []model.EventTime {
	out := make([]model.EventTime, 0, len(times))
	for _, t := range times {
		t.Label = strings.TrimSpace(t.Label)
		t.Start = strings.TrimSpace(t.Start)
		t.End = strings.TrimSpace(t.End)
		if t.Start == "" {
			continue
		}
		out = append(out, t)
	}
	return out
}

// withinAnyEventTime reports whether now falls inside one of the ranges. An empty
// list is unbounded (true) - the same reading an empty availability window gets
// everywhere else in the app: nothing was stated, so nothing is excluded.
func withinAnyEventTime(times []model.EventTime, now time.Time) bool {
	if len(times) == 0 {
		return true
	}
	for _, t := range times {
		if withinWindow(t.Start, t.End, now) {
			return true
		}
	}
	return false
}

// raffleRunning reports whether a raffle assigned to a stall should be offered on
// the public map: it must be open (not closed) AND inside its availability window.
// A closed raffle - or one whose window hasn't opened yet - is left off rather
// than pointing a visitor at a page they can't enter.
func raffleRunning(r store.StallRaffle, now time.Time) bool {
	if r.Status == "closed" {
		return false
	}
	return withinWindow(r.AvailableFrom, r.AvailableTo, now)
}

// -- Admin: list / detail -----------------------------------------------------

// handleFestivalMapsList returns every map (admin only), with stall counts.
//
//	Endpoint:  GET /api/festival-maps
//	Auth:      admin, or a user granted festival-map
func (s *Server) handleFestivalMapsList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalMap) {
		return
	}
	maps, err := s.store.ListFestivalMaps()
	if err != nil {
		writeInternalError(w, "list festival maps", err)
		return
	}
	writeJSON(w, http.StatusOK, model.FestivalMapsResponse{Maps: maps})
}

// handleFestivalMapDetail returns one map with its stalls.
//
//	Endpoint:  GET /api/festival-maps/{id}
//	Auth:      admin, or a user granted festival-map
func (s *Server) handleFestivalMapDetail(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalMap) {
		return
	}
	id, ok := pathInt64(w, r, "id", "festival map")
	if !ok {
		return
	}
	m, err := s.store.GetFestivalMap(id)
	if err != nil {
		writeInternalError(w, "get festival map", err)
		return
	}
	if m == nil {
		writeError(w, http.StatusNotFound, "Festival map not found")
		return
	}
	writeJSON(w, http.StatusOK, model.FestivalMapDetailResponse{Map: *m})
}

// -- Admin: CRUD --------------------------------------------------------------

// festivalMapWriteRequest is the JSON body for creating (POST /api/festival-maps)
// or replacing (PUT /api/festival-maps/{id}) a map. The id comes from the path on
// PUT, and the status is set through PATCH rather than here.
type festivalMapWriteRequest struct {
	Title       string                `json:"title"`
	Slug        string                `json:"slug"`
	Description string                `json:"description"`
	Times       []model.EventTime     `json:"times"`
	MapImage    string                `json:"map_image"`
	Stalls      []model.FestivalStall `json:"stalls"`
}

// sanitizeOccupants normalizes a pitch's occupants and drops the ones that say
// nothing at all - no title, no affiliate. An empty row is what the form leaves
// behind when someone adds an occupant and thinks better of it; storing it would
// put a nameless second line on the plan.
func sanitizeOccupants(in []model.FestivalStallOccupant) []model.FestivalStallOccupant {
	out := make([]model.FestivalStallOccupant, 0, len(in))
	for _, o := range in {
		o.Title = strings.TrimSpace(o.Title)
		o.AffiliateName = ""
		o.StallType = model.NormalizeStallType(o.StallType)
		o.TypeLabel = strings.TrimSpace(o.TypeLabel)
		o.Times = sanitizeEventTimes(o.Times)
		// Only drop a row that ARRIVED with no identity - the blank repeater the
		// form leaves behind when someone adds an occupant and thinks better of it.
		// A stored occupant (id > 0) that has lost its identity is a different
		// thing: an affiliate it named was deleted underneath it, and discarding it
		// here would silently take the whole pitch with it on the next unrelated
		// save, since a pitch left with no occupants is dropped below. Removal is
		// expressed by leaving the occupant OUT of the array, not by blanking it,
		// so nothing legitimate depends on the old behavior.
		if o.ID == 0 && o.Title == "" && o.AffiliateID == nil {
			continue
		}
		out = append(out, o)
	}
	return out
}

// mapFromRequest builds a sanitized model.FestivalMap (sans ID/status) from a
// request: placements clamped into the map box, colors reduced to a hex or
// nothing, and each occupant's type normalized to a known value.
//
// A pitch left with NO occupants is dropped: a shape on the plan that names
// nobody is a coloured box a visitor can click for an empty panel.
func mapFromRequest(req festivalMapWriteRequest, title string) *model.FestivalMap {
	stalls := make([]model.FestivalStall, 0, len(req.Stalls))
	for _, st := range req.Stalls {
		st.Shape = model.NormalizeStallShape(st.Shape)
		st.Color = sanitizeColor(st.Color)
		// Validated like the fill: it reaches CSS as a custom property, so only the
		// "#rrggbb" the picker produces is accepted and anything else becomes "".
		st.SelectionColor = sanitizeColor(st.SelectionColor)
		st.TextColor = sanitizeColor(st.TextColor)
		st.Placement = sanitizePlacement(st.Placement)
		st.Occupants = sanitizeOccupants(st.Occupants)
		if len(st.Occupants) == 0 {
			continue
		}
		stalls = append(stalls, st)
	}
	return &model.FestivalMap{
		Title:       title,
		Slug:        normalizeMapSlug(req.Slug),
		Description: req.Description,
		Times:       sanitizeEventTimes(req.Times),
		MapImage:    strings.TrimSpace(req.MapImage),
		Stalls:      stalls,
	}
}

// handleFestivalMapCreate creates a map (stalls inline). A new map always starts
// in progress - publishing is a deliberate, separate step.
//
//	Endpoint:  POST /api/festival-maps
//	Auth:      admin, or a user granted festival-map
//	Response:  201 {"map": FestivalMap}
func (s *Server) handleFestivalMapCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalMap) {
		return
	}
	req, err := readJSON[festivalMapWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "Title is required")
		return
	}
	m := mapFromRequest(req, title)
	if msg := slugError(m.Slug); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if !s.checkMapSlugUnique(w, m.Slug, 0) {
		return
	}
	m.Status = model.MapStatusInProgress
	id, err := s.store.CreateFestivalMap(m)
	if err != nil {
		writeInternalError(w, "create festival map", err)
		return
	}
	m.ID = id
	writeJSON(w, http.StatusCreated, model.FestivalMapResponse{Map: *m})
}

// handleFestivalMapUpdate replaces a map's editable fields (stalls inline).
// Status is preserved - use PATCH to publish/close.
//
//	Endpoint:  PUT /api/festival-maps/{id}
//	Auth:      admin, or a user granted festival-map
//	Response:  200 {"ok": true}
func (s *Server) handleFestivalMapUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalMap) {
		return
	}
	id, ok := pathInt64(w, r, "id", "festival map")
	if !ok {
		return
	}
	req, err := readJSON[festivalMapWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "Title is required")
		return
	}
	m := mapFromRequest(req, title)
	m.ID = id
	if msg := slugError(m.Slug); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if !s.checkMapSlugUnique(w, m.Slug, id) {
		return
	}
	// Authoritative for the pitches only when the request actually carried a
	// "stalls" key - see the note in UpdateStampRally. An omitted key decodes to a
	// nil slice and leaves the map's pitches alone; an explicit [] still clears
	// them, so deliberately emptying a map keeps working.
	if err := s.store.UpdateFestivalMap(m, req.Stalls != nil); err != nil {
		// A stall id that isn't on this map means a stale editor (someone deleted
		// the pitch meanwhile) or a spoofed body - the caller's problem, not ours,
		// and the save was rejected whole rather than applied across two maps.
		if errors.Is(err, store.ErrStallNotOnMap) {
			writeError(w, http.StatusBadRequest,
				"This map changed since you opened it. Reload the map and reapply your edit.")
			return
		}
		writeInternalError(w, "update festival map", err)
		return
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}

// handleFestivalMapDelete deletes a map and its stalls. Any stamp rally that was
// linked to it survives, with its stamps falling back to their affiliates.
//
//	Endpoint:  DELETE /api/festival-maps/{id}
//	Auth:      admin, or a user granted festival-map
//	Response:  204 No Content
func (s *Server) handleFestivalMapDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalMap) {
		return
	}
	id, ok := pathInt64(w, r, "id", "festival map")
	if !ok {
		return
	}
	if _, err := s.store.DeleteFestivalMap(id); err != nil {
		writeInternalError(w, "delete festival map", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// festivalMapStatusRequest is the JSON body for PATCH /api/festival-maps/{id}.
type festivalMapStatusRequest struct {
	Status string `json:"status"`
}

// handleFestivalMapPatch sets a map's publish status. A map with no base image
// can't be published: the stalls are positioned as a share of that image's box,
// so publishing without one would put a floor plan of floating shapes in front of
// the public.
//
//	Endpoint:  PATCH /api/festival-maps/{id}
//	Auth:      admin, or a user granted festival-map
//	Request:   {"status":"in_progress"|"published"|"closed"}
//	Response:  200 {"ok": true, "status": "published"}
func (s *Server) handleFestivalMapPatch(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permFestivalMap) {
		return
	}
	id, ok := pathInt64(w, r, "id", "festival map")
	if !ok {
		return
	}
	req, err := readJSON[festivalMapStatusRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	// Validate rather than normalize. NormalizeMapStatus exists to make sense of a
	// STORED value and falls back to in_progress for anything it doesn't recognize,
	// which is right when reading a row and wrong here: it turned a typo - or a body
	// with no status at all - into a silent unpublish of a live map.
	switch req.Status {
	case model.MapStatusInProgress, model.MapStatusPublished, model.MapStatusClosed:
	default:
		writeError(w, http.StatusBadRequest,
			`Status must be one of "in_progress", "published" or "closed"`)
		return
	}
	status := req.Status
	if status == model.MapStatusPublished {
		m, err := s.store.GetFestivalMap(id)
		if err != nil {
			writeInternalError(w, "get festival map for publish", err)
			return
		}
		if m == nil {
			writeError(w, http.StatusNotFound, "Festival map not found")
			return
		}
		if strings.TrimSpace(m.MapImage) == "" {
			writeError(w, http.StatusBadRequest,
				"Pick a base map image before publishing - the stalls are positioned against it.")
			return
		}
	}
	changed, err := s.store.SetFestivalMapStatus(id, status)
	if err != nil {
		writeInternalError(w, "set festival map status", err)
		return
	}
	if !changed {
		writeError(w, http.StatusNotFound, "Festival map not found")
		return
	}
	writeJSON(w, http.StatusOK, model.StatusResponse{OK: true, Status: status})
}

// -- Public (published maps only) ---------------------------------------------

// handleFestivalMapsPublic lists the published maps.
//
//	Endpoint:  GET /api/festival-maps/public
//	Auth:      public
func (s *Server) handleFestivalMapsPublic(w http.ResponseWriter, r *http.Request) {
	maps, err := s.store.ListPublishedFestivalMaps()
	if err != nil {
		writeInternalError(w, "list published festival maps", err)
		return
	}
	now := time.Now().UTC()
	out := make([]model.PublicFestivalMapSummary, 0, len(maps))
	for i := range maps {
		m := &maps[i]
		out = append(out, model.PublicFestivalMapSummary{
			ID: m.ID, Title: m.Title, Slug: m.Slug, Description: m.Description, Times: m.Times,
			MapImage: m.MapImage, StallCount: m.StallCount,
			IsActive: withinAnyEventTime(m.Times, now),
		})
	}
	writeJSON(w, http.StatusOK, model.PublicFestivalMapsResponse{Maps: out})
}

// resolvePublicMap looks a map up by its shortcode or, failing that, its numeric
// id. Both always work: a link posted before a shortcode existed keeps resolving,
// and adding or changing one never breaks it. A purely-numeric shortcode is
// refused on save (see slugError), so the two forms can never collide.
func (s *Server) resolvePublicMap(idOrSlug string) (*model.FestivalMap, error) {
	if m, err := s.store.GetFestivalMapBySlug(normalizeMapSlug(idOrSlug)); err != nil || m != nil {
		return m, err
	}
	id, err := strconv.ParseInt(idOrSlug, 10, 64)
	if err != nil {
		return nil, nil // neither a known shortcode nor an id - the caller 404s
	}
	return s.store.GetFestivalMap(id)
}

// handleFestivalMapPublic returns one published map with every stall on it, each
// resolved for a visitor: the affiliate card (name, logo, links), whether the
// stall is open right now, and its stamp-rally badge when an open rally linked to
// this map places a stamp on it.
//
// The path segment is the map's shortcode OR its numeric id (see resolvePublicMap).
// An unpublished map answers 404 rather than 403 - a draft's existence isn't
// something the public endpoint should confirm.
//
//	Endpoint:  GET /api/festival-maps/public/{id}
//	Auth:      public
func (s *Server) handleFestivalMapPublic(w http.ResponseWriter, r *http.Request) {
	m, err := s.resolvePublicMap(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeInternalError(w, "get public festival map", err)
		return
	}
	if m == nil || m.Status != model.MapStatusPublished {
		writeError(w, http.StatusNotFound, "Festival map not found")
		return
	}
	id := m.ID

	// The stamps an open linked rally placed on this map's stalls, so each stall
	// can be badged (and show its stamp art) without a second round trip.
	var rallyStamps map[int64]store.MapStallStamp
	rally, err := s.store.GetOpenRallyForMap(id)
	if err != nil {
		writeInternalError(w, "get rally for festival map", err)
		return
	}
	// Not-closed is not the same as running. The query cannot know the time, so the
	// availability window is checked here against the clock - exactly as
	// raffleRunning does for the raffles on this same payload. Without it the public
	// map badged stalls and advertised sign-up for a rally that had not opened yet,
	// or had already finished its window.
	if rally != nil && !withinWindow(rally.AvailableFrom, rally.AvailableTo, time.Now().UTC()) {
		rally = nil
	}
	if rally != nil {
		rallyStamps, err = s.store.ListRallyStampsForMap(rally.ID)
		if err != nil {
			writeInternalError(w, "list rally stamps for festival map", err)
			return
		}
	}

	// Raffles pinned to this map's stalls. Which of them are actually RUNNING is
	// decided below against the clock, not in the query.
	stallRaffles, err := s.store.ListRafflesForMap(id)
	if err != nil {
		writeInternalError(w, "list raffles for festival map", err)
		return
	}

	affiliates, err := s.affiliateCardsByID(m.Stalls)
	if err != nil {
		writeInternalError(w, "load stall affiliates", err)
		return
	}

	now := time.Now().UTC()
	out := model.PublicFestivalMap{
		ID: m.ID, Title: m.Title, Slug: m.Slug, Description: m.Description, Times: m.Times,
		MapImage: m.MapImage, IsActive: withinAnyEventTime(m.Times, now),
		Stalls: make([]model.PublicFestivalStall, 0, len(m.Stalls)),
	}
	if rally != nil {
		out.StampRallyID = rally.ID
		out.StampRallyTitle = rally.Title
		out.StampRallySignup = rally.PublicSignup
	}
	for i := range m.Stalls {
		st := &m.Stalls[i]
		ps := model.PublicFestivalStall{
			ID: st.ID, Shape: st.Shape, Color: st.Color,
			TextColor: st.TextColor, SelectionColor: st.SelectionColor,
			Placement: st.Placement,
			Occupants: make([]model.PublicStallOccupant, 0, len(st.Occupants)),
		}
		for j := range st.Occupants {
			o := &st.Occupants[j]
			po := model.PublicStallOccupant{
				ID: o.ID, Title: o.Title, Description: o.Description,
				EventCarrd: o.EventCarrd,
				StallType:  o.StallType, TypeLabel: o.TypeLabel, Times: o.Times,
				// An occupant that keeps no hours of its own follows the festival's.
				IsOpen: withinAnyEventTime(o.Times, now) && withinAnyEventTime(m.Times, now),
			}
			if o.AffiliateID != nil {
				if card, found := affiliates[*o.AffiliateID]; found {
					po.Affiliate = card
				}
			}
			if stamp, found := rallyStamps[o.ID]; found && rally != nil {
				po.InStampRally = true
				po.RallyID = rally.ID
				po.RallyTitle = rally.Title
				po.StampImage = stamp.Image
			}
			if raffle, found := stallRaffles[o.ID]; found && raffleRunning(raffle, now) {
				po.Raffle = &model.PublicStallRaffle{
					ID: raffle.ID, Title: raffle.Title, PrizeImage: raffle.PrizeImage,
				}
			}
			ps.Occupants = append(ps.Occupants, po)
		}
		out.Stalls = append(out.Stalls, ps)
	}
	writeJSON(w, http.StatusOK, out)
}

// affiliateCardsByID loads the public affiliate card for every affiliate the
// map's occupants reference, keyed by id. One lookup per DISTINCT affiliate, so a
// plan where a dozen occupants belong to the same partner still costs one read.
func (s *Server) affiliateCardsByID(stalls []model.FestivalStall) (map[int64]*model.PublicStallAffiliate, error) {
	cards := make(map[int64]*model.PublicStallAffiliate)
	for i := range stalls {
		for j := range stalls[i].Occupants {
			id := stalls[i].Occupants[j].AffiliateID
			if id == nil {
				continue
			}
			if _, done := cards[*id]; done {
				continue
			}
			a, err := s.store.GetAffiliate(*id)
			if err != nil {
				return nil, err
			}
			if a == nil {
				continue
			}
			cards[*id] = &model.PublicStallAffiliate{
				Name: a.Name, Subtitle: a.Subtitle, Owners: a.Owners,
				Logo: a.Logo, DiscordLink: a.DiscordLink, CarrdLink: a.CarrdLink,
			}
		}
	}
	return cards, nil
}
