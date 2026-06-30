package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// ── Raffle list (public + admin) ────────────────────────────────────────────

// handleRafflesList returns all raffles visible to the requester.
// Admins see all raffles; public users see only open raffles within availability dates.
//
//	Endpoint:  GET /api/raffles
//	Auth:      public (filtered by role)
//	Response:  {"raffles": [...]}
func (s *Server) handleRafflesList(w http.ResponseWriter, r *http.Request) {
	admin := s.isAdmin(r)
	raffles, err := s.store.ListRaffles(admin)
	if err != nil {
		writeInternalError(w, "list raffles", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"raffles": raffles})
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

	resp := map[string]any{"raffle": raffle}

	// Always include total entry count
	totalEntries, err := s.store.CountRaffleEntries(id)
	if err == nil {
		resp["total_entries"] = totalEntries
	}

	// Include entries for admins, or winner entry for public on closed raffles
	if s.isAdmin(r) {
		entries, err := s.store.ListRaffleEntries(id)
		if err != nil {
			writeInternalError(w, "list raffle entries", err)
			return
		}
		resp["entries"] = entries
	} else if raffle.Status == "closed" && raffle.WinnerEntryID != nil {
		// Show the winner entry to public — fetch directly by ID
		entry, err := s.store.GetRaffleEntryByID(*raffle.WinnerEntryID)
		if err == nil && entry != nil {
			resp["winner_entry"] = entry
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// ── Raffle admin actions (create, update, delete) ───────────────────────────

// raffleRequest is the JSON body for POST /api/raffles.
// Action determines the operation: "create", "update", or "delete".
type raffleRequest struct {
	Action             string  `json:"action"`
	ID                 int64   `json:"id"`
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

// handleRafflesAction processes raffle CRUD operations.
//
//	Endpoint:  POST /api/raffles
//	Auth:      admin, or a user granted this page's permission
//	Request:   {"action": "create"|"update"|"delete", ...}
//	Response:  varies by action
func (s *Server) handleRafflesAction(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseRaffles) {
		return
	}

	req, err := readJSON[raffleRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Action {
	case "create":
		title := strings.TrimSpace(req.Title)
		if title == "" {
			writeError(w, http.StatusBadRequest, "Title is required")
			return
		}
		maxEntries := req.MaxEntries
		if maxEntries < 1 {
			maxEntries = 1
		}
		raffle := &model.Raffle{
			Title:              title,
			Description:        req.Description,
			Rules:              req.Rules,
			MaxEntries:         maxEntries,
			SignupInstructions: req.SignupInstructions,
			CostPerEntry:       req.CostPerEntry,
			AvailableFrom:      req.AvailableFrom,
			AvailableTo:        req.AvailableTo,
			PrizeImage:         req.PrizeImage,
		}
		id, err := s.store.CreateRaffle(raffle)
		if err != nil {
			writeInternalError(w, "create raffle", err)
			return
		}
		raffle.ID = id
		raffle.Status = "open"
		writeJSON(w, http.StatusCreated, map[string]any{"raffle": raffle})

	case "update":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Raffle id is required")
			return
		}
		title := strings.TrimSpace(req.Title)
		if title == "" {
			writeError(w, http.StatusBadRequest, "Title is required")
			return
		}
		maxEntries := req.MaxEntries
		if maxEntries < 1 {
			maxEntries = 1
		}
		raffle := &model.Raffle{
			ID:                 req.ID,
			Title:              title,
			Description:        req.Description,
			Rules:              req.Rules,
			MaxEntries:         maxEntries,
			SignupInstructions: req.SignupInstructions,
			CostPerEntry:       req.CostPerEntry,
			AvailableFrom:      req.AvailableFrom,
			AvailableTo:        req.AvailableTo,
			PrizeImage:         req.PrizeImage,
		}
		if err := s.store.UpdateRaffle(raffle); err != nil {
			writeInternalError(w, "update raffle", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case "delete":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Raffle id is required")
			return
		}
		deleted, err := s.store.DeleteRaffle(req.ID)
		if err != nil {
			writeInternalError(w, "delete raffle", err)
			return
		}
		// Prize images are managed centrally on the System → Images page (the
		// "Raffle" category), so the image file is left intact on delete — it may
		// be reused by another raffle.
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: create, update, delete")
	}
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

// raffleEntryRequest is the JSON body for POST /api/raffles/{id}/enter.
type raffleEntryRequest struct {
	CharacterName string `json:"character_name"`
	World         string `json:"world"`
	NumEntries    int    `json:"num_entries"`
}

// handleRaffleEnter processes a public raffle sign-up.
// Validates availability dates, entry limits, and creates or increments the entry.
//
//	Endpoint:  POST /api/raffles/{id}/enter
//	Auth:      public
//	Request:   {"character_name": "...", "world": "...", "num_entries": 1}
//	Response:  {"message": "...", "total_entries": int, "total_cost": float, "signup_instructions": "..."}
func (s *Server) handleRaffleEnter(w http.ResponseWriter, r *http.Request) {
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
		writeJSON(w, http.StatusCreated, map[string]any{
			"message":             "Signed up successfully",
			"total_entries":       newTotal,
			"total_cost":          totalCost,
			"signup_instructions": raffle.SignupInstructions,
		})
	} else {
		writeJSON(w, http.StatusOK, map[string]any{
			"message":             "Entries added successfully",
			"total_entries":       newTotal,
			"total_cost":          totalCost,
			"signup_instructions": raffle.SignupInstructions,
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

// ── Raffle entries admin actions ────────────────────────────────────────────

// raffleEntriesRequest is the JSON body for POST /api/raffles/{id}/entries.
// Action: "add_entry", "mark_paid", "delete_entry", "pick_winner",
// "verify_winner", or "pick_another".
type raffleEntriesRequest struct {
	Action  string `json:"action"`
	EntryID int64  `json:"entry_id"`
	Paid    bool   `json:"paid"`
	// add_entry only: the player to add and how many tickets.
	CharacterName string `json:"character_name"`
	World         string `json:"world"`
	NumEntries    int    `json:"num_entries"`
}

// handleRaffleEntries processes admin actions on raffle entries.
//
//	Endpoint:  POST /api/raffles/{id}/entries
//	Auth:      admin, or a user granted this page's permission
//	Request:   {"action": "mark_paid"|"delete_entry"|"pick_winner"|"verify_winner"|"pick_another", ...}
//	Response:  varies by action
func (s *Server) handleRaffleEntries(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permTeahouseRaffles) {
		return
	}

	idStr := r.PathValue("id")
	raffleID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid raffle ID")
		return
	}

	req, err := readJSON[raffleEntriesRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Action {
	case "add_entry":
		// Admin manually adds a player to an open raffle, optionally already paid.
		// Unlike the public sign-up this skips the availability-window check (an
		// admin can add an entry at any time while the raffle is open) but still
		// enforces the per-person max so the entry data stays consistent.
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
		entryID, _, prevEntries, _, err := s.store.AddOrCreateRaffleEntry(
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
		if err != nil {
			writeInternalError(w, "load added entry", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"entry": entry})

	case "mark_paid":
		if req.EntryID <= 0 {
			writeError(w, http.StatusBadRequest, "Entry id is required")
			return
		}
		if err := s.store.SetRaffleEntryPaid(req.EntryID, req.Paid); err != nil {
			writeInternalError(w, "mark entry paid", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case "delete_entry":
		if req.EntryID <= 0 {
			writeError(w, http.StatusBadRequest, "Entry id is required")
			return
		}
		deleted, err := s.store.DeleteRaffleEntry(req.EntryID)
		if err != nil {
			writeInternalError(w, "delete raffle entry", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})

	case "pick_winner":
		winner, err := s.store.PickRaffleWinner(raffleID)
		if err != nil {
			writeInternalError(w, "pick raffle winner", err)
			return
		}
		if winner == nil {
			writeError(w, http.StatusBadRequest, "No paid entries to pick from")
			return
		}
		// Set as pending winner
		if err := s.store.SetRaffleWinner(raffleID, &winner.ID); err != nil {
			writeInternalError(w, "set raffle winner", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"winner": winner})

	case "verify_winner":
		// Finalize: close the raffle with the current winner
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
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "closed"})

	case "pick_another":
		// Clear current winner and pick again
		if err := s.store.SetRaffleWinner(raffleID, nil); err != nil {
			writeInternalError(w, "clear raffle winner", err)
			return
		}
		winner, err := s.store.PickRaffleWinner(raffleID)
		if err != nil {
			writeInternalError(w, "pick another winner", err)
			return
		}
		if winner == nil {
			writeError(w, http.StatusBadRequest, "No paid entries to pick from")
			return
		}
		if err := s.store.SetRaffleWinner(raffleID, &winner.ID); err != nil {
			writeInternalError(w, "set another winner", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"winner": winner})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: add_entry, mark_paid, delete_entry, pick_winner, verify_winner, pick_another")
	}
}

// Raffle prize images are uploaded and managed centrally on the System → Images
// page (the "Raffle" category → images/raffles). The raffle editor's picker
// reads that category via GET /api/images; there is no per-raffle upload or
// cleanup here anymore.
