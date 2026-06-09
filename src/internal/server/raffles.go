package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"app-suite/internal/model"
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
//	Auth:      admin
//	Request:   {"action": "create"|"update"|"delete", ...}
//	Response:  varies by action
func (s *Server) handleRafflesAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	req, err := readJSON[raffleRequest](r)
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
		// Fetch raffle to get prize image path before deletion
		raffle, err := s.store.GetRaffle(req.ID)
		if err != nil {
			writeInternalError(w, "get raffle for delete", err)
			return
		}
		deleted, err := s.store.DeleteRaffle(req.ID)
		if err != nil {
			writeInternalError(w, "delete raffle", err)
			return
		}
		// Delete prize image file if it exists
		if deleted && raffle != nil {
			s.removeUploadedImage(raffle.PrizeImage)
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: create, update, delete")
	}
}

// ── Raffle entry (public sign-up) ───────────────────────────────────────────

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

	req, err := readJSON[raffleEntryRequest](r)
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

	// Check availability dates
	now := time.Now().UTC()
	if raffle.AvailableFrom != "" {
		from, err := time.Parse("2006-01-02T15:04", raffle.AvailableFrom)
		if err == nil && now.Before(from) {
			writeError(w, http.StatusBadRequest, "This raffle is not yet open for entries")
			return
		}
	}
	if raffle.AvailableTo != "" {
		to, err := time.Parse("2006-01-02T15:04", raffle.AvailableTo)
		if err == nil && now.After(to) {
			writeError(w, http.StatusBadRequest, "This raffle is no longer accepting entries")
			return
		}
	}

	// Check if character+world already has entries
	existing, err := s.store.GetRaffleEntry(raffleID, charName, world)
	if err != nil {
		writeInternalError(w, "get raffle entry", err)
		return
	}

	if existing != nil {
		// Check if adding entries would exceed max
		newTotal := existing.NumEntries + req.NumEntries
		if newTotal > raffle.MaxEntries {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("Cannot add %d entries. You already have %d of %d max entries.",
					req.NumEntries, existing.NumEntries, raffle.MaxEntries))
			return
		}
		if err := s.store.AddRaffleEntries(existing.ID, req.NumEntries); err != nil {
			writeInternalError(w, "add raffle entries", err)
			return
		}
		totalCost := float64(newTotal) * raffle.CostPerEntry
		writeJSON(w, http.StatusOK, map[string]any{
			"message":             "Entries added successfully",
			"total_entries":       newTotal,
			"total_cost":          totalCost,
			"signup_instructions": raffle.SignupInstructions,
		})
	} else {
		// New entry — check max entries
		if req.NumEntries > raffle.MaxEntries {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("Number of entries cannot exceed %d", raffle.MaxEntries))
			return
		}
		if _, err := s.store.CreateRaffleEntry(raffleID, charName, world, req.NumEntries); err != nil {
			writeInternalError(w, "create raffle entry", err)
			return
		}
		totalCost := float64(req.NumEntries) * raffle.CostPerEntry
		writeJSON(w, http.StatusCreated, map[string]any{
			"message":             "Signed up successfully",
			"total_entries":       req.NumEntries,
			"total_cost":          totalCost,
			"signup_instructions": raffle.SignupInstructions,
		})
	}
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
//	Auth:      admin
//	Request:   {"action": "mark_paid"|"delete_entry"|"pick_winner"|"verify_winner"|"pick_another", ...}
//	Response:  varies by action
func (s *Server) handleRaffleEntries(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	idStr := r.PathValue("id")
	raffleID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid raffle ID")
		return
	}

	req, err := readJSON[raffleEntriesRequest](r)
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

		existing, err := s.store.GetRaffleEntry(raffleID, charName, world)
		if err != nil {
			writeInternalError(w, "get raffle entry for add", err)
			return
		}

		var entryID int64
		if existing != nil {
			newTotal := existing.NumEntries + req.NumEntries
			if newTotal > raffle.MaxEntries {
				writeError(w, http.StatusBadRequest,
					fmt.Sprintf("Cannot add %d entries. They already have %d of %d max entries.",
						req.NumEntries, existing.NumEntries, raffle.MaxEntries))
				return
			}
			if err := s.store.AddRaffleEntries(existing.ID, req.NumEntries); err != nil {
				writeInternalError(w, "add raffle entries", err)
				return
			}
			entryID = existing.ID
		} else {
			if req.NumEntries > raffle.MaxEntries {
				writeError(w, http.StatusBadRequest,
					fmt.Sprintf("Number of entries cannot exceed %d", raffle.MaxEntries))
				return
			}
			entryID, err = s.store.CreateRaffleEntry(raffleID, charName, world, req.NumEntries)
			if err != nil {
				writeInternalError(w, "create raffle entry", err)
				return
			}
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
			writeInternalError(w, "verify raffle winner", fmt.Errorf("get raffle: %v", err))
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

// removeUploadedImage deletes a previously uploaded prize image, but only if
// the resolved path stays within the raffles upload directory. This guards
// against a malicious or accidental prize_image value (e.g. "../../etc/x")
// causing deletion of files outside the upload area.
func (s *Server) removeUploadedImage(webPath string) {
	if webPath == "" {
		return
	}
	uploadDir, err := filepath.Abs(filepath.Join(s.webRoot, "images", "raffles"))
	if err != nil {
		return
	}
	target, err := filepath.Abs(filepath.Join(s.webRoot, filepath.Clean(webPath)))
	if err != nil {
		return
	}
	// Ensure target is inside uploadDir (and not the dir itself).
	if target == uploadDir || !strings.HasPrefix(target, uploadDir+string(os.PathSeparator)) {
		return
	}
	_ = os.Remove(target)
}

// ── Raffle image upload ─────────────────────────────────────────────────────

// handleRaffleUpload handles multipart image uploads for raffle prize images.
// Validates file extension (jpg/png/webp/gif) and size (max 5 MB).
//
//	Endpoint:  POST /api/raffles/upload
//	Auth:      admin
//	Request:   multipart form with "image" field
//	Response:  {"path": "images/raffles/raffle_....ext"}
func (s *Server) handleRaffleUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	// Limit upload to 5 MB
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)

	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Image upload failed (max 5MB)")
		return
	}
	defer file.Close()

	// Validate extension
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" && ext != ".gif" {
		writeError(w, http.StatusBadRequest, "Only jpg, png, webp, and gif images are allowed")
		return
	}

	// Ensure directory exists
	dir := filepath.Join(s.webRoot, "images", "raffles")
	if err := os.MkdirAll(dir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create upload directory")
		return
	}

	// Generate unique filename
	filename := fmt.Sprintf("raffle_%d%s", time.Now().UnixNano(), ext)
	destPath := filepath.Join(dir, filename)

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

	// Return the web-accessible path
	webPath := "images/raffles/" + filename
	writeJSON(w, http.StatusOK, map[string]any{"path": webPath})
}
