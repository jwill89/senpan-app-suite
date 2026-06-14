package server

import (
	"fmt"
	"net/http"
	"strings"
)

// patternRequest is the JSON body for POST /api/patterns.
// Action determines the operation: "create", "delete", "rename", "reorder",
// "set_category", or "bulk_reorder".
type patternRequest struct {
	Action      string   `json:"action"`
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	PatternData [][]bool `json:"pattern_data"`
	Direction   string   `json:"direction"`
	CategoryID  int64    `json:"category_id"`
	OrderedIDs  []int    `json:"ordered_ids"`
}

// handlePatternsList returns all patterns grouped by category.
//
//	Endpoint:  GET /api/patterns
//	Auth:      admin
//	Response:  {"patterns": [...], "categories": [...]}
func (s *Server) handlePatternsList(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	patterns, err := s.store.ListPatterns()
	if err != nil {
		writeInternalError(w, "patterns", err)
		return
	}
	categories, err := s.store.ListPatternCategories()
	if err != nil {
		writeInternalError(w, "patterns", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"patterns": patterns, "categories": categories})
}

// handlePatternsAction processes pattern CRUD and reorder operations.
//
//	Endpoint:    POST /api/patterns
//	Auth:        admin
//	Request:     {"action": "create"|"delete"|"rename"|"reorder"|"set_category"|"bulk_reorder", ...}
//	Response:    varies by action
//	Broadcasts:  patterns_update
func (s *Server) handlePatternsAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	req, err := readJSON[patternRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Action {
	case "create":
		name := strings.TrimSpace(req.Name)
		if name == "" || len(req.PatternData) != 5 {
			writeError(w, http.StatusBadRequest, "Provide a name and a 5×5 pattern_data array")
			return
		}
		// Validate each row is length 5.
		for _, row := range req.PatternData {
			if len(row) != 5 {
				writeError(w, http.StatusBadRequest, "Provide a name and a 5×5 pattern_data array")
				return
			}
		}

		// Check for duplicate pattern
		dup, err := s.store.FindDuplicatePattern(req.PatternData)
		if err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		if dup != nil {
			writeError(w, http.StatusConflict,
				fmt.Sprintf("Duplicate pattern! Matches \"%s\" in category \"%s\"", dup.Name, dup.CategoryName))
			return
		}

		id, err := s.store.SavePattern(name, req.PatternData, req.CategoryID)
		if err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"pattern": map[string]any{
				"id":           id,
				"name":         name,
				"pattern_data": req.PatternData,
				"category_id":  req.CategoryID,
			},
		})
		s.broadcastPatterns()

	case "delete":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Pattern id is required")
			return
		}
		deleted, err := s.store.DeletePattern(req.ID)
		if err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
		s.broadcastPatterns()

	case "rename":
		name := strings.TrimSpace(req.Name)
		if req.ID <= 0 || name == "" {
			writeError(w, http.StatusBadRequest, "Provide pattern id and new name")
			return
		}
		renamed, err := s.store.RenamePattern(req.ID, name)
		if err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"renamed": renamed})
		s.broadcastPatterns()

	case "reorder":
		if req.ID <= 0 || (req.Direction != "up" && req.Direction != "down") {
			writeError(w, http.StatusBadRequest, "Provide pattern id and direction (up or down)")
			return
		}
		if _, err := s.store.MovePattern(req.ID, req.Direction); err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		patterns, err := s.store.ListPatterns()
		if err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		categories, err := s.store.ListPatternCategories()
		if err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"patterns": patterns, "categories": categories})
		s.broadcastPatterns()

	case "set_category":
		if req.ID <= 0 || req.CategoryID <= 0 {
			writeError(w, http.StatusBadRequest, "Provide pattern id and category_id")
			return
		}
		if err := s.store.SetPatternCategory(req.ID, req.CategoryID); err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		s.broadcastPatterns()

	case "bulk_reorder":
		if req.CategoryID <= 0 || len(req.OrderedIDs) == 0 {
			writeError(w, http.StatusBadRequest, "Provide category_id and ordered_ids array")
			return
		}
		if err := s.store.BulkReorderPatterns(req.CategoryID, req.OrderedIDs); err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		patterns, err := s.store.ListPatterns()
		if err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		categories, err := s.store.ListPatternCategories()
		if err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"patterns": patterns, "categories": categories})
		s.broadcastPatterns()

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: create, delete, rename, reorder, set_category, bulk_reorder")
	}
}

// ── Pattern Category handlers ───────────────────────────────────────────────

// categoryRequest is the JSON body for POST /api/pattern-categories.
// Action determines the operation: "create", "delete", "rename", "reorder", or "bulk_reorder".
type categoryRequest struct {
	Action     string  `json:"action"`
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Direction  string  `json:"direction"`
	OrderedIDs []int64 `json:"ordered_ids"`
}

// handleCategoriesList returns all pattern categories ordered by sort_order.
//
//	Endpoint:  GET /api/pattern-categories
//	Auth:      admin
//	Response:  {"categories": [...]}
func (s *Server) handleCategoriesList(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	categories, err := s.store.ListPatternCategories()
	if err != nil {
		writeInternalError(w, "patterns", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": categories})
}

// handleCategoriesAction processes category CRUD and reorder operations.
//
//	Endpoint:    POST /api/pattern-categories
//	Auth:        admin
//	Request:     {"action": "create"|"delete"|"rename"|"reorder"|"bulk_reorder", ...}
//	Response:    varies by action
//	Broadcasts:  patterns_update
func (s *Server) handleCategoriesAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	req, err := readJSON[categoryRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Action {
	case "create":
		name := strings.TrimSpace(req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "Category name is required")
			return
		}
		id, err := s.store.CreatePatternCategory(name)
		if err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": name})
		s.broadcastPatterns()

	case "rename":
		name := strings.TrimSpace(req.Name)
		if req.ID <= 0 || name == "" {
			writeError(w, http.StatusBadRequest, "Provide category id and new name")
			return
		}
		renamed, err := s.store.RenamePatternCategory(req.ID, name)
		if err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"renamed": renamed})
		s.broadcastPatterns()

	case "delete":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Category id is required")
			return
		}
		deleted, err := s.store.DeletePatternCategory(req.ID)
		if err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		if !deleted {
			writeError(w, http.StatusBadRequest, "Cannot delete the last category")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
		s.broadcastPatterns()

	case "reorder":
		if req.ID <= 0 || (req.Direction != "up" && req.Direction != "down") {
			writeError(w, http.StatusBadRequest, "Provide category id and direction (up or down)")
			return
		}
		if _, err := s.store.MovePatternCategory(req.ID, req.Direction); err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		categories, err := s.store.ListPatternCategories()
		if err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"categories": categories})
		s.broadcastPatterns()

	case "bulk_reorder":
		if len(req.OrderedIDs) == 0 {
			writeError(w, http.StatusBadRequest, "Provide ordered_ids array")
			return
		}
		if err := s.store.BulkReorderCategories(req.OrderedIDs); err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		categories, err := s.store.ListPatternCategories()
		if err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"categories": categories})
		s.broadcastPatterns()

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: create, delete, rename, reorder, bulk_reorder")
	}
}
