package server

import (
	"net/http"
	"strconv"

	"app-suite/internal/model"
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

	writeJSON(w, http.StatusOK, model.WinnersLogResponse{
		Entries: entries,
		Total:   total,
		Page:    page,
		PerPage: perPage,
	})
}

// handleWinnersLogDelete removes one winners-log entry. Deleting a non-existent
// entry is a no-op success (idempotent).
//
//	Endpoint:  DELETE /api/winners-log/{id}
//	Auth:      admin, or a user granted this page's permission
//	Response:  204 No Content
func (s *Server) handleWinnersLogDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoWinnersLog) {
		return
	}
	id, ok := pathInt64(w, r, "id", "entry")
	if !ok {
		return
	}
	if _, err := s.store.DeleteWinnerLogEntry(id); err != nil {
		writeInternalError(w, "delete winners log entry", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleWinnersLogDeleteAll clears the entire winners log, reporting how many
// rows were removed (per the project's bulk-delete convention).
//
//	Endpoint:  DELETE /api/winners-log/all
//	Auth:      admin, or a user granted this page's permission
//	Response:  200 {"deleted": N}
func (s *Server) handleWinnersLogDeleteAll(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoWinnersLog) {
		return
	}
	deleted, err := s.store.DeleteAllWinnersLog()
	if err != nil {
		writeInternalError(w, "delete all winners log", err)
		return
	}
	writeJSON(w, http.StatusOK, model.DeletedCountResponse{Deleted: deleted})
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

	writeJSON(w, http.StatusOK, model.FrequentWinnersResponse{Winners: winners})
}
