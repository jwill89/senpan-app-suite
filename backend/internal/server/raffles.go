package server

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// -- Raffle list (public + admin) --------------------------------------------

// raffleStaff reports whether the request may see the privileged raffle view
// (all raffles, entry lists, winner_name/paid_total aggregates). Raffle writes
// gate on permTeahouseRaffles, so the reads align to the same permission -
// otherwise a non-admin granted teahouse-raffles could manage a raffle but not
// see its paid totals. Admins hold every permission.
func (s *Server) raffleStaff(r *http.Request) bool {
	u := s.currentUser(r)
	return u != nil && (u.IsAdmin || userHasPermission(u, permTeahouseRaffles))
}

// handleRafflesList returns all raffles visible to the requester.
// Raffle staff see all raffles; public users see only open raffles within
// availability dates.
//
//	Endpoint:  GET /api/raffles
//	Auth:      public (filtered by role)
//	Response:  {"raffles": [...]}
func (s *Server) handleRafflesList(w http.ResponseWriter, r *http.Request) {
	staff := s.raffleStaff(r)
	raffles, err := s.store.ListRaffles(staff)
	if err != nil {
		writeInternalError(w, "list raffles", err)
		return
	}
	writeJSON(w, http.StatusOK, model.RafflesResponse{Raffles: raffles})
}

// -- Raffle detail (public + admin) ------------------------------------------

// handleRaffleDetail returns a single raffle with entries (admin) or winner info (public).
//
//	Endpoint:  GET /api/raffles/{id}
//	Auth:      public (response varies by role)
//	Response:  {"raffle": Raffle, "total_entries": int, "entries": [...] (admin), "winner_entry": Entry (public/closed)}
func (s *Server) handleRaffleDetail(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid raffle ID")
		return
	}

	raffle, err := s.store.GetRaffle(id)
	if err != nil {
		writeInternalError(w, "get raffle", err)
		return
	}
	if raffle == nil {
		writeError(w, http.StatusNotFound, "Raffle not found")
		return
	}

	staff := s.raffleStaff(r)

	// Non-staff may only view a currently-public raffle: an open one inside its
	// availability window (same predicate as the public list) or a closed one
	// (winner announcement). Otherwise 404 so a not-yet-open raffle's details
	// can't be read by guessing its ID.
	if !staff && !raffleIsPubliclyViewable(raffle) {
		writeError(w, http.StatusNotFound, "Raffle not found")
		return
	}

	resp := model.RaffleDetailResponse{Raffle: *raffle}

	// Always include total entry count
	totalEntries, err := s.store.CountRaffleEntries(id)
	if err == nil {
		resp.TotalEntries = &totalEntries
	}

	// Include entries for staff, or winner entry for public on closed raffles
	if staff {
		entries, err := s.store.ListRaffleEntries(id)
		if err != nil {
			writeInternalError(w, "list raffle entries", err)
			return
		}
		resp.Entries = &entries
	} else if raffle.Status == "closed" && raffle.WinnerEntryID != nil {
		// Show the winner entry to public - fetch directly by ID, minus the
		// settlement columns (what they paid for and what was waived is staff-only;
		// see RaffleEntry.PublicView).
		entry, err := s.store.GetRaffleEntryByID(*raffle.WinnerEntryID)
		if err == nil && entry != nil {
			public := entry.PublicView()
			resp.WinnerEntry = &public
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// -- Raffle create / update / delete -----------------------------------------

// raffleWriteRequest is the JSON body for creating (POST /api/raffles) or
// replacing (PUT /api/raffles/{id}) a raffle. The id comes from the path on PUT.
type raffleWriteRequest struct {
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	Rules              string    `json:"rules"`
	MaxEntries         int       `json:"max_entries"`
	SignupInstructions string    `json:"signup_instructions"`
	EntryMode          string    `json:"entry_mode"`
	CostPerEntry       float64   `json:"cost_per_entry"`
	TierCosts          []float64 `json:"tier_costs"`
	AvailableFrom      string    `json:"available_from"`
	AvailableTo        string    `json:"available_to"`
	PrizeImage         string    `json:"prize_image"`
	PayImage           string    `json:"pay_image"`
	FestivalMapID      *int64    `json:"festival_map_id"` // optional Festival Map the raffle belongs to
	OccupantID         *int64    `json:"occupant_id"`     // optional stall on that map
}

// resolveRaffleStall settles a raffle's Festival Map link and its assigned stall
// against what actually exists, writing the error response itself and returning
// false when the link can't be honored.
//
// Mirrors the stamp rally's resolveMapStalls: a raffle that names no map has its
// stall cleared (naming one would claim a festival link it doesn't have), and a
// stall that isn't on the linked map is dropped rather than failing the save -
// the raffle is still a perfectly good raffle, it just isn't pinned to a pitch.
func (s *Server) resolveRaffleStall(w http.ResponseWriter, raffle *model.Raffle) bool {
	if raffle.FestivalMapID == nil {
		raffle.OccupantID = nil
		return true
	}
	m, err := s.store.GetFestivalMap(*raffle.FestivalMapID)
	if err != nil {
		writeInternalError(w, "get festival map for raffle", err)
		return false
	}
	if m == nil {
		writeError(w, http.StatusBadRequest, "That festival map no longer exists")
		return false
	}
	if raffle.OccupantID == nil {
		return true
	}
	for i := range m.Stalls {
		for j := range m.Stalls[i].Occupants {
			if m.Stalls[i].Occupants[j].ID == *raffle.OccupantID {
				return true
			}
		}
	}
	raffle.OccupantID = nil
	return true
}

// maxRaffleEntries caps the per-player allowance, and with it the custom-cost
// ladder (whose length IS the allowance). A raffle hands out tickets, not a shop
// inventory, so a hundred per person is already far past any real use. The bound
// keeps a typo (or a scripted client) from storing an allowance nothing can
// render sanely, or an unbounded JSON array on the row.
const maxRaffleEntries = 100

// maxEntrantFieldLen bounds the free-text identity fields a PUBLIC sign-up sends
// (character name, world). An FFXIV character name plus world is well under this;
// the cap exists so an unauthenticated caller can't store an unbounded string that
// then has to render on the staff entry list and inside a Discord embed. Matches
// the public custom-card request's limit (see handleCardRequest).
const maxEntrantFieldLen = 60

// validate checks a raffle write request: a non-empty title, plus whichever cost
// the entry mode actually uses. Every price must be finite and non-negative - a
// NaN/Inf or negative cost would corrupt every total_cost the sign-up flow
// reports. max_entries is floored to 1 (or pinned to the ladder) in toRaffle, so
// it needs no separate check here.
//
// An unknown or absent entry_mode normalizes to "single", so a client written
// before entry modes existed still validates the way it always did. It returns a
// user-facing error message, or "" when the request is valid.
func (req raffleWriteRequest) validate() string {
	if strings.TrimSpace(req.Title) == "" {
		return "Title is required"
	}
	switch model.NormalizeRaffleMode(req.EntryMode) {
	case model.RaffleModeDetails:
		// Details-only raffles record no money at all; any cost sent with one is
		// dropped in toRaffle rather than rejected.
		return ""
	case model.RaffleModeCustom:
		if len(req.TierCosts) == 0 {
			return "Add at least one entry cost"
		}
		// The ladder length IS the allowance in this mode, so it takes the same cap.
		if len(req.TierCosts) > maxRaffleEntries {
			return fmt.Sprintf("A raffle can have at most %d entry costs", maxRaffleEntries)
		}
		for i, c := range req.TierCosts {
			if math.IsNaN(c) || math.IsInf(c, 0) || c < 0 {
				return fmt.Sprintf("Cost for entry %d must be a non-negative number", i+1)
			}
		}
		return ""
	default:
		if math.IsNaN(req.CostPerEntry) || math.IsInf(req.CostPerEntry, 0) || req.CostPerEntry < 0 {
			return "Cost per entry must be a non-negative number"
		}
		return ""
	}
}

// validateMaxEntries bounds the per-player allowance for the modes that set it
// directly. It is a rejection rather than a silent clamp: an admin who typed 1000
// meant something, and quietly storing 100 instead would surface later as a cap
// nobody chose. (The <1 floor stays silent in toRaffle - 0 has no other sensible
// reading.) Custom mode derives max_entries from its ladder, which validate()
// bounds instead.
func (req raffleWriteRequest) validateMaxEntries() string {
	if model.NormalizeRaffleMode(req.EntryMode) == model.RaffleModeCustom {
		return ""
	}
	if req.MaxEntries > maxRaffleEntries {
		return fmt.Sprintf("Max entries per person cannot exceed %d", maxRaffleEntries)
	}
	return ""
}

// toRaffle builds a model.Raffle from the request, flooring max_entries to 1 and
// keeping only the cost fields its entry mode uses - so switching a raffle from
// custom back to single doesn't leave a stale ladder behind that later reads as
// the live price.
//
// In custom mode the ladder IS the allowance: max_entries is pinned to its
// length, because a 4th ticket in a 3-rung ladder would have no price.
func (req raffleWriteRequest) toRaffle(id int64) *model.Raffle {
	maxEntries := req.MaxEntries
	if maxEntries < 1 {
		maxEntries = 1
	}
	mode := model.NormalizeRaffleMode(req.EntryMode)
	raffle := &model.Raffle{
		ID:                 id,
		Title:              strings.TrimSpace(req.Title),
		Description:        req.Description,
		Rules:              req.Rules,
		MaxEntries:         maxEntries,
		SignupInstructions: req.SignupInstructions,
		EntryMode:          mode,
		// Empty, not nil: the create response echoes this struct straight back,
		// and a nil slice would marshal as `null` where every later read of the
		// same raffle returns `[]` (the store decodes it that way).
		TierCosts:     []float64{},
		AvailableFrom: req.AvailableFrom,
		AvailableTo:   req.AvailableTo,
		PrizeImage:    req.PrizeImage,
		PayImage:      strings.TrimSpace(req.PayImage),
		FestivalMapID: req.FestivalMapID,
		OccupantID:    req.OccupantID,
	}
	switch mode {
	case model.RaffleModeCustom:
		raffle.TierCosts = req.TierCosts
		raffle.MaxEntries = len(req.TierCosts)
	case model.RaffleModeSingle:
		raffle.CostPerEntry = req.CostPerEntry
	}
	return raffle
}

// handleRaffleCreate creates a raffle.
//
//	Endpoint:  POST /api/raffles
//	Auth:      permission:teahouse-raffles
//	Response:  201 {"raffle": Raffle}
func (s *Server) handleRaffleCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseRaffles) {
		return
	}
	req, err := readJSON[raffleWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if msg := req.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if msg := req.validateMaxEntries(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	raffle := req.toRaffle(0)
	if !s.resolveRaffleStall(w, raffle) {
		return
	}
	id, err := s.store.CreateRaffle(raffle)
	if err != nil {
		writeInternalError(w, "create raffle", err)
		return
	}
	raffle.ID = id
	raffle.Status = "open"
	writeJSON(w, http.StatusCreated, model.RaffleResponse{Raffle: *raffle})
}

// handleRaffleUpdate replaces a raffle's editable fields (status/winner are not
// editable here and are preserved).
//
//	Endpoint:  PUT /api/raffles/{id}
//	Auth:      permission:teahouse-raffles
//	Response:  200 {"raffle": Raffle}
func (s *Server) handleRaffleUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseRaffles) {
		return
	}
	id, ok := pathInt64(w, r, "id", "raffle")
	if !ok {
		return
	}
	req, err := readJSON[raffleWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if msg := req.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if msg := req.validateMaxEntries(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	updated := req.toRaffle(id)
	if !s.resolveRaffleStall(w, updated) {
		return
	}
	if err := s.store.UpdateRaffle(updated); err != nil {
		writeInternalError(w, "update raffle", err)
		return
	}
	raffle, err := s.store.GetRaffle(id)
	if err != nil || raffle == nil {
		writeInternalError(w, "load updated raffle", err)
		return
	}
	writeJSON(w, http.StatusOK, model.RaffleResponse{Raffle: *raffle})
}

// handleRaffleDelete deletes a raffle. Prize images are managed centrally on
// System -> Images (the "Raffle" category), so the file is left intact - it may
// be reused by another raffle.
//
//	Endpoint:  DELETE /api/raffles/{id}
//	Auth:      permission:teahouse-raffles
//	Response:  204 No Content
func (s *Server) handleRaffleDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseRaffles) {
		return
	}
	id, ok := pathInt64(w, r, "id", "raffle")
	if !ok {
		return
	}
	if _, err := s.store.DeleteRaffle(id); err != nil {
		writeInternalError(w, "delete raffle", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// -- Raffle entry (public sign-up) -------------------------------------------

// parseRaffleTime parses a raffle availability timestamp into a UTC instant.
// New values are stored as UTC RFC-3339 (e.g. "2026-06-13T20:00:00.000Z");
// legacy values are naive "2006-01-02T15:04" strings, which we interpret as UTC
// to stay consistent with the SQL availability filter. Returns the instant and
// whether parsing succeeded (false for empty/unparseable input -> no constraint).
func parseRaffleTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse("2006-01-02T15:04", s); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

// raffleIsPubliclyViewable reports whether a raffle should be visible to
// non-admins via the detail endpoint: a closed raffle (winner announcement), or
// an open raffle currently inside its availability window. This mirrors the
// public list filter in Store.ListRaffles so detail can't reveal a scheduled/
// not-yet-open raffle that the list hides.
func raffleIsPubliclyViewable(raffle *model.Raffle) bool {
	if raffle.Status == "closed" {
		return true
	}
	if raffle.Status != "open" {
		return false
	}
	now := time.Now().UTC()
	if from, ok := parseRaffleTime(raffle.AvailableFrom); ok && now.Before(from) {
		return false
	}
	if to, ok := parseRaffleTime(raffle.AvailableTo); ok && now.After(to) {
		return false
	}
	return true
}

// raffleEntryRequest is the JSON body for POST /api/raffles/{id}/enter.
type raffleEntryRequest struct {
	CharacterName  string `json:"character_name"`
	World          string `json:"world"`
	NumEntries     int    `json:"num_entries"`
	TurnstileToken string `json:"turnstile_token"` // Cloudflare Turnstile token (when enabled)
}

// handleRaffleEnter processes a public raffle sign-up.
// Validates availability dates, entry limits, and creates or increments the entry.
//
//	Endpoint:  POST /api/raffles/{id}/enter
//	Auth:      public
//	Request:   {"character_name": "...", "world": "...", "num_entries": 1}
//	Response:  {"message": "...", "total_entries": int, "total_cost": float, "signup_instructions": "..."}
func (s *Server) handleRaffleEnter(w http.ResponseWriter, r *http.Request) {
	// Public endpoint: throttle per IP so a bot can't flood a raffle's entry
	// table under arbitrary character names. Every attempt counts against it.
	ip := clientIP(r)
	if s.raffleLimiter.isLimited(ip) {
		slog.Warn("raffle entry rate limited", "ip", ip)
		writeError(w, http.StatusTooManyRequests, "Too many entries. Please try again later.")
		return
	}
	s.raffleLimiter.recordFailure(ip)

	idStr := r.PathValue("id")
	raffleID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid raffle ID")
		return
	}

	req, err := readJSON[raffleEntryRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Bot check: when Cloudflare Turnstile is configured, require a valid one-time
	// token before recording an entry (entry counts drive the weighted winner
	// pick, so bot-flooded entries would skew the odds).
	if s.turnstileEnabled() && !s.verifyTurnstile(r.Context(), req.TurnstileToken, ip) {
		slog.Warn("turnstile verification failed (raffle entry)", "ip", ip)
		writeError(w, http.StatusForbidden, "Bot check failed. Please try again.")
		return
	}

	charName := strings.TrimSpace(req.CharacterName)
	world := strings.TrimSpace(req.World)
	if charName == "" || world == "" {
		writeError(w, http.StatusBadRequest, "Character name and world are required")
		return
	}
	// Same cap the public custom-card request applies. This endpoint is public and
	// its values are rendered on the staff entry list and into a Discord embed, so
	// without a bound a single sign-up could store a megabyte of "name" - the JSON
	// body limit was the only thing standing in the way.
	if len(charName) > maxEntrantFieldLen || len(world) > maxEntrantFieldLen {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("Character name and world must each be %d characters or fewer", maxEntrantFieldLen))
		return
	}
	if req.NumEntries < 1 {
		req.NumEntries = 1
	}
	// Ceiling as well as floor. maxRaffleEntries already bounds every raffle's
	// own allowance, so nothing legitimate asks for more, and rejecting here
	// keeps an absurd count from reaching the cap arithmetic at all.
	if req.NumEntries > maxRaffleEntries {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("Number of entries cannot exceed %d", maxRaffleEntries))
		return
	}

	raffle, err := s.store.GetRaffle(raffleID)
	if err != nil {
		writeInternalError(w, "get raffle for entry", err)
		return
	}
	if raffle == nil {
		writeError(w, http.StatusNotFound, "Raffle not found")
		return
	}
	if raffle.Status != "open" {
		writeError(w, http.StatusBadRequest, "This raffle is no longer accepting entries")
		return
	}
	// A details-only raffle publishes its instructions and takes sign-ups
	// elsewhere; it has no public entry form, so a POST here is always a client
	// out of step with the raffle (or one poking at the endpoint directly).
	if !raffle.AcceptsSignups() {
		writeError(w, http.StatusBadRequest, "This raffle does not take sign-ups here. Follow its sign-up instructions.")
		return
	}

	// Check availability dates. Stored timestamps are UTC (RFC-3339 with 'Z' for
	// new values; legacy naive strings are interpreted as UTC), so we compare
	// against the current UTC instant - timezone-correct regardless of where the
	// raffle was created.
	now := time.Now().UTC()
	if from, ok := parseRaffleTime(raffle.AvailableFrom); ok && now.Before(from) {
		writeError(w, http.StatusBadRequest, "This raffle is not yet open for entries")
		return
	}
	if to, ok := parseRaffleTime(raffle.AvailableTo); ok && now.After(to) {
		writeError(w, http.StatusBadRequest, "This raffle is no longer accepting entries")
		return
	}

	// Record the entries atomically: the store enforces the per-player cap and the
	// add-vs-create decision inside one write transaction, so two simultaneous
	// sign-ups for the same character+world can't both pass a stale count check and
	// exceed the cap (or create duplicate rows).
	entryID, newTotal, prevEntries, created, err := s.store.AddOrCreateRaffleEntry(
		raffleID, charName, world, req.NumEntries, raffle.MaxEntries)
	if errors.Is(err, store.ErrRaffleEntryLimit) {
		if prevEntries > 0 {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("Cannot add %d entries. You already have %d of %d max entries.",
					req.NumEntries, prevEntries, raffle.MaxEntries))
		} else {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("Number of entries cannot exceed %d", raffle.MaxEntries))
		}
		return
	}
	if err != nil {
		writeInternalError(w, "record raffle entry", err)
		return
	}

	// What they owe is the sticker price MINUS whatever this character has already
	// settled on this raffle - a returning entrant must not be quoted for tickets
	// they have already paid for. A brand-new entry has nothing settled, so the two
	// figures coincide. A read failure falls back to the full price: over-quoting is
	// recoverable (staff see the real balance), silently under-quoting is not.
	totalCost := raffle.EntryCost(newTotal)
	amountDue := totalCost
	if entry, err := s.store.GetRaffleEntryByID(entryID); err == nil && entry != nil {
		amountDue = raffle.AmountOutstanding(*entry)
	}
	message := "Entries added successfully"
	status := http.StatusOK
	if created {
		message = "Signed up successfully"
		status = http.StatusCreated
	}
	writeJSON(w, status, model.RaffleEnterResponse{
		Message:            message,
		TotalEntries:       newTotal,
		TotalCost:          totalCost,
		AmountDue:          amountDue,
		SignupInstructions: raffle.SignupInstructions,
	})

	// A sign-up mutates the admin-visible entry list + counts, but this is the
	// *public* entry path, so it's excluded from the adminMutationResource
	// middleware (which matches the admin ".../entries" suffix, not ".../enter").
	// Broadcast the "raffles" signal explicitly so an admin viewing the raffle
	// detail sees the new entry appear live (the refetch re-applies the guard).
	// Only success reaches here - every validation/error path above returns first.
	s.broadcastResourceChanged("raffles")
}

// -- Raffle entry lookup (public) --------------------------------------------

// raffleLookupRequest is the JSON body for POST /api/raffles/{id}/lookup. The
// name travels in the body rather than a query string so it stays out of proxy
// and access logs.
type raffleLookupRequest struct {
	Name string `json:"name"`
}

const (
	// raffleLookupMinQuery is the shortest search accepted. Entrants are looking
	// for their own name, which they know; a one-character query is not that, it
	// is a request for the entrant list.
	raffleLookupMinQuery = 2
	// raffleLookupLimit bounds one response. Past it the reply says so, so a broad
	// search is asked to narrow rather than shown a clipped list as if it were all.
	raffleLookupLimit = 50
)

// handleRaffleEntryLookup answers "have I already entered this raffle?" so a
// returning entrant can reproduce the exact name they used - entries merge on
// character+world, and a different spelling silently creates a second entry that
// splits their tickets.
//
// This is deliberately more open than the stamp-rally lookup, which matches whole
// names to keep participation private: here a partial match is the point, and the
// operator has accepted that a raffle's entrant list is public. What it still
// withholds is money - the amount waived on an entry is a private arrangement
// (see model.RaffleLookupEntry).
//
//	Endpoint:  POST /api/raffles/{id}/lookup
//	Auth:      public
//	Request:   {"name": "ari"}
//	Response:  {"entries": [...], "truncated": bool}
func (s *Server) handleRaffleEntryLookup(w http.ResponseWriter, r *http.Request) {
	// Its OWN limiter, not the entry limiter: searching is what somebody does
	// right before signing up, and sharing a budget would let a few searches lock
	// them out of the entry they were searching in order to get right.
	ip := clientIP(r)
	if s.raffleLookupLimiter.isLimited(ip) {
		slog.Warn("raffle entry lookup rate limited", "ip", ip)
		writeError(w, http.StatusTooManyRequests, "Too many searches. Please try again later.")
		return
	}
	s.raffleLookupLimiter.recordFailure(ip)

	raffleID, ok := pathInt64(w, r, "id", "raffle")
	if !ok {
		return
	}
	req, err := readJSON[raffleLookupRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	name := strings.TrimSpace(req.Name)
	if len([]rune(name)) < raffleLookupMinQuery {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("Enter at least %d characters of the name you signed up with", raffleLookupMinQuery))
		return
	}

	// Only a raffle the caller could already see: a scheduled or hidden one must
	// not become searchable just because its id was guessed.
	raffle, err := s.store.GetRaffle(raffleID)
	if err != nil {
		writeInternalError(w, "get raffle for lookup", err)
		return
	}
	if raffle == nil || (!s.raffleStaff(r) && !raffleIsPubliclyViewable(raffle)) {
		writeError(w, http.StatusNotFound, "Raffle not found")
		return
	}

	entries, truncated, err := s.store.LookupRaffleEntries(raffleID, name, raffleLookupLimit)
	if err != nil {
		writeInternalError(w, "look up raffle entries", err)
		return
	}
	writeJSON(w, http.StatusOK, model.RaffleLookupResponse{Entries: entries, Truncated: truncated})
}

// -- Raffle entries (admin) --------------------------------------------------

// raffleEntryAddRequest is the JSON body for POST /api/raffles/{id}/entries.
type raffleEntryAddRequest struct {
	CharacterName string  `json:"character_name"`
	World         string  `json:"world"`
	NumEntries    int     `json:"num_entries"`
	Paid          bool    `json:"paid"`
	AmountWaived  float64 `json:"amount_waived"`
}

// validWaiver reports whether a waived amount can be recorded: finite and not
// negative. A NaN/Inf would poison every collected-gil total that ever reads the
// row, and a negative "waiver" would silently invent income.
func validWaiver(amount float64) bool {
	return !math.IsNaN(amount) && !math.IsInf(amount, 0) && amount >= 0
}

// handleRaffleEntryAdd adds an entry to an open raffle (admin). Unlike the public
// sign-up it skips the availability-window check (an admin can add at any time
// while the raffle is open) but still enforces the per-person max.
//
//	Endpoint:  POST /api/raffles/{id}/entries
//	Auth:      permission:teahouse-raffles
//	Response:  201 {"entry": RaffleEntry}
func (s *Server) handleRaffleEntryAdd(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseRaffles) {
		return
	}
	raffleID, ok := pathInt64(w, r, "id", "raffle")
	if !ok {
		return
	}
	req, err := readJSON[raffleEntryAddRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	charName := strings.TrimSpace(req.CharacterName)
	world := strings.TrimSpace(req.World)
	if charName == "" || world == "" {
		writeError(w, http.StatusBadRequest, "Character name and world are required")
		return
	}
	if req.NumEntries < 1 {
		req.NumEntries = 1
	}
	if req.NumEntries > maxRaffleEntries {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("Number of entries cannot exceed %d", maxRaffleEntries))
		return
	}
	if !validWaiver(req.AmountWaived) {
		writeError(w, http.StatusBadRequest, "Amount waived must be a non-negative number")
		return
	}

	raffle, err := s.store.GetRaffle(raffleID)
	if err != nil {
		writeInternalError(w, "get raffle for add entry", err)
		return
	}
	if raffle == nil {
		writeError(w, http.StatusNotFound, "Raffle not found")
		return
	}
	if raffle.Status != "open" {
		writeError(w, http.StatusBadRequest, "This raffle is no longer accepting entries")
		return
	}

	// Same atomic cap-enforced write as the public enter path, so an admin and a
	// player adding entries for the same character+world at once can't race past
	// the max.
	entryID, _, prevEntries, created, err := s.store.AddOrCreateRaffleEntry(
		raffleID, charName, world, req.NumEntries, raffle.MaxEntries)
	if errors.Is(err, store.ErrRaffleEntryLimit) {
		if prevEntries > 0 {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("Cannot add %d entries. They already have %d of %d max entries.",
					req.NumEntries, prevEntries, raffle.MaxEntries))
		} else {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("Number of entries cannot exceed %d", raffle.MaxEntries))
		}
		return
	}
	if err != nil {
		writeInternalError(w, "record raffle entry", err)
		return
	}

	// Settle right away when requested (never un-settles an existing entry). The
	// tickets just added are outstanding by definition, so this always applies.
	if req.Paid {
		if _, err := s.store.SetRaffleEntryPaid(entryID, true, 0, req.AmountWaived); err != nil {
			writeInternalError(w, "mark added entry paid", err)
			return
		}
	}

	entry, err := s.store.GetRaffleEntryByID(entryID)
	if err != nil || entry == nil {
		writeInternalError(w, "load added entry", err)
		return
	}
	// 201 when a new entry row was created; 200 when merged into an existing one.
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, model.RaffleEntryResponse{Entry: *entry})
}

// raffleEntryPatchRequest is the JSON body for PATCH /api/raffles/{id}/entries/{entryId}.
type raffleEntryPatchRequest struct {
	Paid bool `json:"paid"`
	// How many of the entry's tickets this settlement covers. Omitted or 0 means
	// all of them (the common counter case); a smaller number records a PART
	// payment and leaves the entry reading as partial. Ignored when paid is false.
	PaidEntries int `json:"paid_entries"`
	// Gil forgiven as part of THIS settlement. It is added to whatever the entry
	// has already had waived, never substituted for it - a player whose first
	// entry was free and who later buys two more keeps both waivers. Ignored when
	// paid is false, which clears the row outright.
	AmountWaived float64 `json:"amount_waived"`
}

// handleRaffleEntryPatch records or clears a settlement on an entry: paid=true
// settles paid_entries tickets (all of them when it is omitted) and adds
// amount_waived to the running waiver, paid=false resets the row to nothing
// settled and nothing waived.
//
//	Endpoint:  PATCH /api/raffles/{id}/entries/{entryId}
//	Auth:      permission:teahouse-raffles
//	Response:  200 {"entry": RaffleEntry}
func (s *Server) handleRaffleEntryPatch(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseRaffles) {
		return
	}
	raffleID, ok := pathInt64(w, r, "id", "raffle")
	if !ok {
		return
	}
	entryID, ok := pathInt64(w, r, "entryId", "entry")
	if !ok {
		return
	}
	req, err := readJSON[raffleEntryPatchRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	// Scope the entry to the raffle in the path: load it first and confirm it
	// belongs to {id}, so a valid entry id from a DIFFERENT raffle can't be
	// mutated by pairing it with any raffle id (IDOR).
	entry, err := s.store.GetRaffleEntryByID(entryID)
	if err != nil {
		writeInternalError(w, "load entry", err)
		return
	}
	if entry == nil || entry.RaffleID != raffleID {
		writeError(w, http.StatusNotFound, "Entry not found")
		return
	}
	if !validWaiver(req.AmountWaived) {
		writeError(w, http.StatusBadRequest, "Amount waived must be a non-negative number")
		return
	}
	if req.PaidEntries < 0 {
		writeError(w, http.StatusBadRequest, "Entries paid for cannot be negative")
		return
	}
	if _, err := s.store.SetRaffleEntryPaid(entryID, req.Paid, req.PaidEntries, req.AmountWaived); err != nil {
		writeInternalError(w, "mark entry paid", err)
		return
	}
	// Re-read rather than patching the local copy: the store derives
	// paid_entries and accumulates amount_waived, so only the row knows the
	// result.
	updated, err := s.store.GetRaffleEntryByID(entryID)
	if err != nil || updated == nil {
		writeInternalError(w, "load settled entry", err)
		return
	}
	writeJSON(w, http.StatusOK, model.RaffleEntryResponse{Entry: *updated})
}

// handleRaffleEntryDelete removes a raffle entry.
//
//	Endpoint:  DELETE /api/raffles/{id}/entries/{entryId}
//	Auth:      permission:teahouse-raffles
//	Response:  204 No Content
func (s *Server) handleRaffleEntryDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseRaffles) {
		return
	}
	raffleID, ok := pathInt64(w, r, "id", "raffle")
	if !ok {
		return
	}
	entryID, ok := pathInt64(w, r, "entryId", "entry")
	if !ok {
		return
	}
	// Scope the entry to the raffle in the path: confirm it belongs to {id}
	// before deleting, so an entry id from a DIFFERENT raffle can't be deleted
	// by pairing it with any raffle id (IDOR).
	entry, err := s.store.GetRaffleEntryByID(entryID)
	if err != nil {
		writeInternalError(w, "load entry for delete", err)
		return
	}
	if entry == nil || entry.RaffleID != raffleID {
		writeError(w, http.StatusNotFound, "Entry not found")
		return
	}
	if _, err := s.store.DeleteRaffleEntry(entryID); err != nil {
		writeInternalError(w, "delete raffle entry", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// -- Raffle winner commands --------------------------------------------------

// pickRaffleWinner picks a random entry as the pending winner and returns it, or
// writes a 400 when there is nothing to pick from. Shared by pick-winner and
// pick-another (which clears the current winner first).
//
// A raffle that charges for tickets draws from PAID entries only. A details-only
// raffle never collects payment through the app - staff record entries from a
// sign-up run elsewhere - so it draws from all of them.
func (s *Server) pickRaffleWinner(w http.ResponseWriter, raffleID int64) {
	raffle, err := s.store.GetRaffle(raffleID)
	if err != nil {
		writeInternalError(w, "get raffle for winner pick", err)
		return
	}
	if raffle == nil {
		writeError(w, http.StatusNotFound, "Raffle not found")
		return
	}
	paidOnly := raffle.AcceptsSignups()
	winner, err := s.store.PickRaffleWinner(raffleID, paidOnly)
	if err != nil {
		writeInternalError(w, "pick raffle winner", err)
		return
	}
	if winner == nil {
		if paidOnly {
			writeError(w, http.StatusBadRequest, "No paid entries to pick from")
		} else {
			writeError(w, http.StatusBadRequest, "No entries to pick from")
		}
		return
	}
	if err := s.store.SetRaffleWinner(raffleID, &winner.ID); err != nil {
		writeInternalError(w, "set raffle winner", err)
		return
	}
	writeJSON(w, http.StatusOK, model.RaffleWinnerResponse{Winner: *winner})
}

// handleRafflePickWinner selects a random paid entry as the pending winner.
//
//	Endpoint:  POST /api/raffles/{id}/pick-winner
//	Auth:      permission:teahouse-raffles
//	Response:  200 {"winner": RaffleEntry}
func (s *Server) handleRafflePickWinner(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseRaffles) {
		return
	}
	raffleID, ok := pathInt64(w, r, "id", "raffle")
	if !ok {
		return
	}
	s.pickRaffleWinner(w, raffleID)
}

// handleRafflePickAnother clears the pending winner and re-picks.
//
//	Endpoint:  POST /api/raffles/{id}/pick-another
//	Auth:      permission:teahouse-raffles
//	Response:  200 {"winner": RaffleEntry}
func (s *Server) handleRafflePickAnother(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseRaffles) {
		return
	}
	raffleID, ok := pathInt64(w, r, "id", "raffle")
	if !ok {
		return
	}
	if err := s.store.SetRaffleWinner(raffleID, nil); err != nil {
		writeInternalError(w, "clear raffle winner", err)
		return
	}
	s.pickRaffleWinner(w, raffleID)
}

// setRaffleStatus applies a status change and responds with {ok, status}. Shared
// by the close and reopen verb handlers.
//
// This is the counterpart to verify-winner, which also closes but only as the last
// step of confirming a winner. Plenty of raffles never reach that: a details-only
// one collects nothing through the app, and any raffle can simply draw no entries.
// Without a plain close those sit open forever, still listed to the public.
func (s *Server) setRaffleStatus(w http.ResponseWriter, r *http.Request, status string) {
	if !s.requirePermission(w, r, permTeahouseRaffles) {
		return
	}
	id, ok := pathInt64(w, r, "id", "raffle")
	if !ok {
		return
	}
	raffle, err := s.store.GetRaffle(id)
	if err != nil {
		writeInternalError(w, "get raffle for status", err)
		return
	}
	if raffle == nil {
		writeError(w, http.StatusNotFound, "Raffle not found")
		return
	}
	if err := s.store.SetRaffleStatus(id, status); err != nil {
		writeInternalError(w, "set raffle status", err)
		return
	}
	writeJSON(w, http.StatusOK, model.StatusResponse{OK: true, Status: status})
}

// handleRaffleClose closes a raffle without picking a winner - for one that drew
// no entries, or a details-only raffle whose draw happened elsewhere. Any winner
// already recorded is left alone, so this never rewrites a result.
//
//	Endpoint:  POST /api/raffles/{id}/close
//	Auth:      permission:teahouse-raffles
//	Response:  200 {"ok": true, "status": "closed"}
func (s *Server) handleRaffleClose(w http.ResponseWriter, r *http.Request) {
	s.setRaffleStatus(w, r, "closed")
}

// handleRaffleReopen puts a closed raffle back on the public list - the undo for
// a close, including one done by mistake.
//
//	Endpoint:  POST /api/raffles/{id}/reopen
//	Auth:      permission:teahouse-raffles
//	Response:  200 {"ok": true, "status": "open"}
func (s *Server) handleRaffleReopen(w http.ResponseWriter, r *http.Request) {
	s.setRaffleStatus(w, r, "open")
}

// handleRaffleVerifyWinner finalizes the pending winner and closes the raffle.
//
//	Endpoint:  POST /api/raffles/{id}/verify-winner
//	Auth:      permission:teahouse-raffles
//	Response:  200 {"ok": true, "status": "closed"}
func (s *Server) handleRaffleVerifyWinner(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseRaffles) {
		return
	}
	raffleID, ok := pathInt64(w, r, "id", "raffle")
	if !ok {
		return
	}
	raffle, err := s.store.GetRaffle(raffleID)
	if err != nil || raffle == nil {
		writeInternalError(w, "verify raffle winner", fmt.Errorf("get raffle: %w", err))
		return
	}
	if raffle.WinnerEntryID == nil {
		writeError(w, http.StatusBadRequest, "No winner selected to verify")
		return
	}
	if err := s.store.SetRaffleStatus(raffleID, "closed"); err != nil {
		writeInternalError(w, "close raffle", err)
		return
	}
	writeJSON(w, http.StatusOK, model.StatusResponse{OK: true, Status: "closed"})
}

// Raffle prize images are uploaded and managed centrally on the System -> Images
// page (the "Raffle" category -> images/raffles). The raffle editor's picker
// reads that category via GET /api/images; there is no per-raffle upload or
// cleanup here anymore.
