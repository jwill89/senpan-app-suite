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

// ── Raffle list (public + admin) ────────────────────────────────────────────

// raffleStaff reports whether the request may see the privileged raffle view
// (all raffles, entry lists, winner_name/paid_total aggregates). Raffle writes
// gate on permTeahouseRaffles, so the reads align to the same permission —
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

// ── Raffle detail (public + admin) ──────────────────────────────────────────

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
		// Show the winner entry to public — fetch directly by ID
		entry, err := s.store.GetRaffleEntryByID(*raffle.WinnerEntryID)
		if err == nil && entry != nil {
			resp.WinnerEntry = entry
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// ── Raffle create / update / delete ─────────────────────────────────────────

// raffleWriteRequest is the JSON body for creating (POST /api/raffles) or
// replacing (PUT /api/raffles/{id}) a raffle. The id comes from the path on PUT.
type raffleWriteRequest struct {
	Title              string  `json:"title"`
	Description        string  `json:"description"`
	Rules              string  `json:"rules"`
	MaxEntries         int     `json:"max_entries"`
	SignupInstructions string  `json:"signup_instructions"`
	CostPerEntry       float64 `json:"cost_per_entry"`
	AvailableFrom      string  `json:"available_from"`
	AvailableTo        string  `json:"available_to"`
	PrizeImage         string  `json:"prize_image"`
}

// validate checks a raffle write request: a non-empty title, and a
// cost_per_entry that is finite and non-negative (a NaN/Inf or negative cost
// would corrupt every total_cost the sign-up flow reports). max_entries is
// floored to 1 in toRaffle, so it needs no separate check here. It returns a
// user-facing error message, or "" when the request is valid.
func (req raffleWriteRequest) validate() string {
	if strings.TrimSpace(req.Title) == "" {
		return "Title is required"
	}
	if math.IsNaN(req.CostPerEntry) || math.IsInf(req.CostPerEntry, 0) || req.CostPerEntry < 0 {
		return "Cost per entry must be a non-negative number"
	}
	return ""
}

// toRaffle builds a model.Raffle from the request, flooring max_entries to 1.
func (req raffleWriteRequest) toRaffle(id int64) *model.Raffle {
	maxEntries := req.MaxEntries
	if maxEntries < 1 {
		maxEntries = 1
	}
	return &model.Raffle{
		ID:                 id,
		Title:              strings.TrimSpace(req.Title),
		Description:        req.Description,
		Rules:              req.Rules,
		MaxEntries:         maxEntries,
		SignupInstructions: req.SignupInstructions,
		CostPerEntry:       req.CostPerEntry,
		AvailableFrom:      req.AvailableFrom,
		AvailableTo:        req.AvailableTo,
		PrizeImage:         req.PrizeImage,
	}
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
	raffle := req.toRaffle(0)
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
	if err := s.store.UpdateRaffle(req.toRaffle(id)); err != nil {
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
// System → Images (the "Raffle" category), so the file is left intact — it may
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

// ── Raffle entry (public sign-up) ───────────────────────────────────────────

// parseRaffleTime parses a raffle availability timestamp into a UTC instant.
// New values are stored as UTC RFC-3339 (e.g. "2026-06-13T20:00:00.000Z");
// legacy values are naive "2006-01-02T15:04" strings, which we interpret as UTC
// to stay consistent with the SQL availability filter. Returns the instant and
// whether parsing succeeded (false for empty/unparseable input → no constraint).
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
	if req.NumEntries < 1 {
		req.NumEntries = 1
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

	// Check availability dates. Stored timestamps are UTC (RFC-3339 with 'Z' for
	// new values; legacy naive strings are interpreted as UTC), so we compare
	// against the current UTC instant — timezone-correct regardless of where the
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
	_, newTotal, prevEntries, created, err := s.store.AddOrCreateRaffleEntry(
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

	totalCost := float64(newTotal) * raffle.CostPerEntry
	if created {
		writeJSON(w, http.StatusCreated, model.RaffleEnterResponse{
			Message:            "Signed up successfully",
			TotalEntries:       newTotal,
			TotalCost:          totalCost,
			SignupInstructions: raffle.SignupInstructions,
		})
	} else {
		writeJSON(w, http.StatusOK, model.RaffleEnterResponse{
			Message:            "Entries added successfully",
			TotalEntries:       newTotal,
			TotalCost:          totalCost,
			SignupInstructions: raffle.SignupInstructions,
		})
	}

	// A sign-up mutates the admin-visible entry list + counts, but this is the
	// *public* entry path, so it's excluded from the adminMutationResource
	// middleware (which matches the admin ".../entries" suffix, not ".../enter").
	// Broadcast the "raffles" signal explicitly so an admin viewing the raffle
	// detail sees the new entry appear live (the refetch re-applies the guard).
	// Only success reaches here — every validation/error path above returns first.
	s.broadcastResourceChanged("raffles")
}

// ── Raffle entries (admin) ──────────────────────────────────────────────────

// raffleEntryAddRequest is the JSON body for POST /api/raffles/{id}/entries.
type raffleEntryAddRequest struct {
	CharacterName string `json:"character_name"`
	World         string `json:"world"`
	NumEntries    int    `json:"num_entries"`
	Paid          bool   `json:"paid"`
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

	// Mark paid right away when requested (never un-marks an existing entry).
	if req.Paid {
		if err := s.store.SetRaffleEntryPaid(entryID, true); err != nil {
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
}

// handleRaffleEntryPatch updates an entry's paid flag.
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
	if err := s.store.SetRaffleEntryPaid(entryID, req.Paid); err != nil {
		writeInternalError(w, "mark entry paid", err)
		return
	}
	entry.Paid = req.Paid
	writeJSON(w, http.StatusOK, model.RaffleEntryResponse{Entry: *entry})
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

// ── Raffle winner commands ──────────────────────────────────────────────────

// pickRaffleWinner picks a random paid entry as the pending winner and returns it,
// or writes a 400 when there are no paid entries. Shared by pick-winner and
// pick-another (which clears the current winner first).
func (s *Server) pickRaffleWinner(w http.ResponseWriter, raffleID int64) {
	winner, err := s.store.PickRaffleWinner(raffleID)
	if err != nil {
		writeInternalError(w, "pick raffle winner", err)
		return
	}
	if winner == nil {
		writeError(w, http.StatusBadRequest, "No paid entries to pick from")
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

// Raffle prize images are uploaded and managed centrally on the System → Images
// page (the "Raffle" category → images/raffles). The raffle editor's picker
// reads that category via GET /api/images; there is no per-raffle upload or
// cleanup here anymore.
