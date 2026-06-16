package server

import (
	"net/http"
	"strconv"
)

// handleWinnersLog returns a paginated list of winners log entries.
//
//	Endpoint:  GET /api/winners-log?page=1&per_page=25&sort=logged_at&dir=desc
//	Auth:      admin, or a user granted this page's permission
//	Params:    page, per_page (1–200), sort (logged_at|card_id|player_name|game_details), dir (asc|desc)
//	Response:  {"entries": [...], "total": int, "page": int, "per_page": int}
func (s *Server) handleWinnersLog(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoWinnersLog) {
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 200 {
		perPage = 25
	}
	sortField := r.URL.Query().Get("sort")
	sortDir := r.URL.Query().Get("dir")

	offset := (page - 1) * perPage
	entries, total, err := s.store.ListWinnersLog(perPage, offset, sortField, sortDir)
	if err != nil {
		writeInternalError(w, "list winners log", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"entries":  entries,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// winnersLogRequest is the JSON body for POST /api/winners-log.
type winnersLogRequest struct {
	Action string `json:"action"`
	ID     int64  `json:"id"` // for "delete"
}

// handleWinnersLogAction deletes one winners-log entry or clears the whole log.
//
//	Endpoint:  POST /api/winners-log
//	Auth:      admin, or a user granted this page's permission
//	Request:   {"action": "delete", "id": N} | {"action": "delete_all"}
//	Response:  {"ok": true}
func (s *Server) handleWinnersLogAction(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoWinnersLog) {
		return
	}
	req, err := readJSON[winnersLogRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Action {
	case "delete":
		if req.ID == 0 {
			writeError(w, http.StatusBadRequest, "Entry id is required")
			return
		}
		if _, err := s.store.DeleteWinnerLogEntry(req.ID); err != nil {
			writeInternalError(w, "delete winners log entry", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case "delete_all":
		if _, err := s.store.DeleteAllWinnersLog(); err != nil {
			writeInternalError(w, "delete all winners log", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: delete, delete_all")
	}
}

// handleFrequentWinners returns players who have won N+ times in the last H hours.
// Thresholds are configurable via app settings (defaults: 3 wins, 12 hours).
//
//	Endpoint:  GET /api/winners-log/frequent
//	Auth:      admin, or a user granted this page's permission
//	Response:  {"winners": [{player_name, win_count}, ...]}
func (s *Server) handleFrequentWinners(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoWinnersLog) {
		return
	}

	threshold := s.getSettingInt("frequent_winner_threshold", 3)
	hours := s.getSettingInt("frequent_winner_hours", 12)

	winners, err := s.store.FrequentWinners(threshold, hours)
	if err != nil {
		writeInternalError(w, "frequent winners", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"winners": winners})
}
