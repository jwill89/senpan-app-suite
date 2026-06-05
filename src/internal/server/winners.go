package server

import (
	"net/http"
	"strconv"
)

// handleWinnersLog returns a paginated list of winners log entries.
//
//	Endpoint:  GET /api/winners-log?page=1&per_page=25&sort=logged_at&dir=desc
//	Auth:      admin
//	Params:    page, per_page (1–200), sort (logged_at|card_id|player_name|game_details), dir (asc|desc)
//	Response:  {"entries": [...], "total": int, "page": int, "per_page": int}
func (s *Server) handleWinnersLog(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
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

// handleFrequentWinners returns players who have won N+ times in the last H hours.
// Thresholds are configurable via app settings (defaults: 3 wins, 12 hours).
//
//	Endpoint:  GET /api/winners-log/frequent
//	Auth:      admin
//	Response:  {"winners": [{player_name, win_count}, ...]}
func (s *Server) handleFrequentWinners(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
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
