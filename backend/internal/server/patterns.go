package server

import (
	"fmt"
	"net/http"
	"strings"

	"app-suite/internal/model"
)

// handlePatternsList returns all patterns grouped by category.
//
//	Endpoint:  GET /api/patterns
//	Auth:      admin, or a user granted this page's permission
//	Response:  {"patterns": [...], "categories": [...]}
func (s *Server) handlePatternsList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoPatterns) {
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
	writeJSON(w, http.StatusOK, model.PatternsResponse{Patterns: patterns, Categories: categories})
}

// patternsAndCategories fetches the full patterns + categories snapshot, the
// shape returned after a (single or bulk) reorder so the client re-renders from
// authoritative order. Writes an internal error and returns ok=false on failure.
func (s *Server) patternsAndCategories(w http.ResponseWriter) (model.PatternsResponse, bool) {
	patterns, err := s.store.ListPatterns()
	if err != nil {
		writeInternalError(w, "patterns", err)
		return model.PatternsResponse{}, false
	}
	categories, err := s.store.ListPatternCategories()
	if err != nil {
		writeInternalError(w, "patterns", err)
		return model.PatternsResponse{}, false
	}
	return model.PatternsResponse{Patterns: patterns, Categories: categories}, true
}

// patternCreateRequest is the JSON body for POST /api/patterns.
type patternCreateRequest struct {
	Name        string   `json:"name"`
	PatternData [][]bool `json:"pattern_data"`
	CategoryID  int64    `json:"category_id"`
}

// handlePatternCreate creates a win pattern.
//
//	Endpoint:    POST /api/patterns
//	Auth:        permission:bingo-patterns
//	Request:     {"name": "...", "pattern_data": [[...]], "category_id": N}
//	Response:    201 PatternCreateResponse
//	Broadcasts:  patterns_update
func (s *Server) handlePatternCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoPatterns) {
		return
	}
	req, err := readJSON[patternCreateRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
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

	// Check for duplicate pattern.
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
	writeJSON(w, http.StatusCreated, model.PatternCreateResponse{
		Pattern: model.CreatedPattern{
			ID:          id,
			Name:        name,
			PatternData: req.PatternData,
			CategoryID:  req.CategoryID,
		},
	})
	s.broadcastPatterns()
}

// handlePatternDelete deletes one pattern by id.
//
//	Endpoint:    DELETE /api/patterns/{id}
//	Auth:        permission:bingo-patterns
//	Response:    204 No Content
//	Broadcasts:  patterns_update
func (s *Server) handlePatternDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoPatterns) {
		return
	}
	id64, ok := pathInt64(w, r, "id", "pattern")
	if !ok {
		return
	}
	if _, err := s.store.DeletePattern(int(id64)); err != nil {
		writeInternalError(w, "patterns", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	s.broadcastPatterns()
}

// patternPatchRequest is the JSON body for PATCH /api/patterns/{id}. All fields
// are pointers so the handler can tell which were supplied and apply only those:
// Name renames, CategoryID moves to a category, Direction (up|down) reorders.
type patternPatchRequest struct {
	Name       *string `json:"name"`
	CategoryID *int64  `json:"category_id"`
	Direction  *string `json:"direction"`
}

// handlePatternPatch partially updates a pattern: rename (name), move to a
// category (category_id), and/or reorder within its category (direction). A
// reorder returns the fresh patterns+categories snapshot (so the client can
// re-render from authoritative order); a pure rename/category change returns
// {"ok": true}.
//
//	Endpoint:    PATCH /api/patterns/{id}
//	Auth:        permission:bingo-patterns
//	Request:     {"name"?: "...", "category_id"?: N, "direction"?: "up"|"down"}
//	Response:    200 PatternsResponse (when direction moved) | 200 OKResponse
//	Broadcasts:  patterns_update
func (s *Server) handlePatternPatch(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoPatterns) {
		return
	}
	id64, ok := pathInt64(w, r, "id", "pattern")
	if !ok {
		return
	}
	id := int(id64)
	req, err := readJSON[patternPatchRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Name == nil && req.CategoryID == nil && req.Direction == nil {
		writeError(w, http.StatusBadRequest, "Provide name, category_id, or direction")
		return
	}

	// Rename + set-category may both be applied in one call.
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "Pattern name cannot be empty")
			return
		}
		if _, err := s.store.RenamePattern(id, name); err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
	}
	if req.CategoryID != nil {
		if *req.CategoryID <= 0 {
			writeError(w, http.StatusBadRequest, "category_id must be a positive id")
			return
		}
		if err := s.store.SetPatternCategory(id, *req.CategoryID); err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
	}

	// A direction move returns the fresh ordered snapshot.
	if req.Direction != nil {
		if *req.Direction != "up" && *req.Direction != "down" {
			writeError(w, http.StatusBadRequest, "direction must be up or down")
			return
		}
		if _, err := s.store.MovePattern(id, *req.Direction); err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		resp, ok := s.patternsAndCategories(w)
		if !ok {
			return
		}
		writeJSON(w, http.StatusOK, resp)
		s.broadcastPatterns()
		return
	}

	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
	s.broadcastPatterns()
}

// patternBulkReorderRequest is the JSON body for POST /api/patterns/reorder.
type patternBulkReorderRequest struct {
	CategoryID int64 `json:"category_id"`
	OrderedIDs []int `json:"ordered_ids"`
}

// handlePatternsReorder persists a new drag-and-drop order for the patterns in a
// category, returning the fresh patterns+categories snapshot.
//
//	Endpoint:    POST /api/patterns/reorder
//	Auth:        permission:bingo-patterns
//	Request:     {"category_id": N, "ordered_ids": [...]}
//	Response:    200 PatternsResponse
//	Broadcasts:  patterns_update
func (s *Server) handlePatternsReorder(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoPatterns) {
		return
	}
	req, err := readJSON[patternBulkReorderRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.CategoryID <= 0 || len(req.OrderedIDs) == 0 {
		writeError(w, http.StatusBadRequest, "Provide category_id and ordered_ids array")
		return
	}
	if err := s.store.BulkReorderPatterns(req.CategoryID, req.OrderedIDs); err != nil {
		writeInternalError(w, "patterns", err)
		return
	}
	resp, ok := s.patternsAndCategories(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, resp)
	s.broadcastPatterns()
}

// ── Pattern Category handlers ───────────────────────────────────────────────

// handleCategoriesList returns all pattern categories ordered by sort_order.
//
//	Endpoint:  GET /api/pattern-categories
//	Auth:      admin, or a user granted this page's permission
//	Response:  {"categories": [...]}
func (s *Server) handleCategoriesList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoPatterns) {
		return
	}

	categories, err := s.store.ListPatternCategories()
	if err != nil {
		writeInternalError(w, "patterns", err)
		return
	}
	writeJSON(w, http.StatusOK, model.CategoriesResponse{Categories: categories})
}

// categoryCreateRequest is the JSON body for POST /api/pattern-categories.
type categoryCreateRequest struct {
	Name string `json:"name"`
}

// handleCategoryCreate creates a pattern category.
//
//	Endpoint:    POST /api/pattern-categories
//	Auth:        permission:bingo-patterns
//	Request:     {"name": "..."}
//	Response:    201 CategoryCreateResponse
//	Broadcasts:  patterns_update
func (s *Server) handleCategoryCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoPatterns) {
		return
	}
	req, err := readJSON[categoryCreateRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
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
	writeJSON(w, http.StatusCreated, model.CategoryCreateResponse{ID: id, Name: name})
	s.broadcastPatterns()
}

// handleCategoryDelete deletes a pattern category. Deleting the last remaining
// category is refused (409) — the store reports deleted=false in that case.
//
//	Endpoint:    DELETE /api/pattern-categories/{id}
//	Auth:        permission:bingo-patterns
//	Response:    204 No Content
//	Broadcasts:  patterns_update
func (s *Server) handleCategoryDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoPatterns) {
		return
	}
	id, ok := pathInt64(w, r, "id", "category")
	if !ok {
		return
	}
	deleted, err := s.store.DeletePatternCategory(id)
	if err != nil {
		writeInternalError(w, "patterns", err)
		return
	}
	if !deleted {
		writeError(w, http.StatusConflict, "Cannot delete the last category")
		return
	}
	w.WriteHeader(http.StatusNoContent)
	s.broadcastPatterns()
}

// categoryPatchRequest is the JSON body for PATCH /api/pattern-categories/{id}.
// Both fields are pointers: Name renames, Direction (up|down) reorders.
type categoryPatchRequest struct {
	Name      *string `json:"name"`
	Direction *string `json:"direction"`
}

// handleCategoryPatch partially updates a category: rename (name) and/or reorder
// (direction). A reorder returns the fresh categories list; a pure rename returns
// {"ok": true}.
//
//	Endpoint:    PATCH /api/pattern-categories/{id}
//	Auth:        permission:bingo-patterns
//	Request:     {"name"?: "...", "direction"?: "up"|"down"}
//	Response:    200 CategoriesResponse (when direction moved) | 200 OKResponse
//	Broadcasts:  patterns_update
func (s *Server) handleCategoryPatch(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoPatterns) {
		return
	}
	id, ok := pathInt64(w, r, "id", "category")
	if !ok {
		return
	}
	req, err := readJSON[categoryPatchRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if req.Name == nil && req.Direction == nil {
		writeError(w, http.StatusBadRequest, "Provide name or direction")
		return
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "Category name is required")
			return
		}
		if _, err := s.store.RenamePatternCategory(id, name); err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
	}

	if req.Direction != nil {
		if *req.Direction != "up" && *req.Direction != "down" {
			writeError(w, http.StatusBadRequest, "direction must be up or down")
			return
		}
		if _, err := s.store.MovePatternCategory(id, *req.Direction); err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		categories, err := s.store.ListPatternCategories()
		if err != nil {
			writeInternalError(w, "patterns", err)
			return
		}
		writeJSON(w, http.StatusOK, model.CategoriesResponse{Categories: categories})
		s.broadcastPatterns()
		return
	}

	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
	s.broadcastPatterns()
}

// categoryBulkReorderRequest is the JSON body for POST /api/pattern-categories/reorder.
type categoryBulkReorderRequest struct {
	OrderedIDs []int64 `json:"ordered_ids"`
}

// handleCategoriesReorder persists a new order for all categories, returning the
// fresh categories list.
//
//	Endpoint:    POST /api/pattern-categories/reorder
//	Auth:        permission:bingo-patterns
//	Request:     {"ordered_ids": [...]}
//	Response:    200 CategoriesResponse
//	Broadcasts:  patterns_update
func (s *Server) handleCategoriesReorder(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoPatterns) {
		return
	}
	req, err := readJSON[categoryBulkReorderRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
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
	writeJSON(w, http.StatusOK, model.CategoriesResponse{Categories: categories})
	s.broadcastPatterns()
}
