package server

import (
	"fmt"
	"net/http"
	"strings"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// handleStylesList returns all styles (without CSS content) and the active style ID.
//
//	Endpoint:  GET /api/styles
//	Auth:      admin, or a user granted this page's permission
//	Response:  {"styles": [...], "active_style_id": "..."}
func (s *Server) handleStylesList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permSystemThemes) {
		return
	}

	styles, err := s.store.ListStyles()
	if err != nil {
		writeInternalError(w, "list styles", err)
		return
	}
	activeID, _ := s.store.GetSetting("active_style_id")
	writeJSON(w, http.StatusOK, model.StylesResponse{
		Styles:        styles,
		ActiveStyleID: activeID,
	})
}

// handleStyleGet returns one style (tokens + generated CSS).
//
//	Endpoint:  GET /api/styles/{id}
//	Auth:      permission:system-themes
//	Response:  200 StyleGetResponse (404 when not found)
func (s *Server) handleStyleGet(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permSystemThemes) {
		return
	}
	id, ok := pathInt64(w, r, "id", "style")
	if !ok {
		return
	}
	style, err := s.store.GetStyle(id)
	if err != nil {
		writeInternalError(w, "get style", err)
		return
	}
	if style == nil {
		writeError(w, http.StatusNotFound, "Style not found")
		return
	}
	writeJSON(w, http.StatusOK, model.StyleGetResponse{Style: *style})
}

// styleWriteRequest is the JSON body for creating (POST /api/styles) or replacing
// (PUT /api/styles/{id}) a style. The id comes from the path on PUT.
type styleWriteRequest struct {
	Name string `json:"name"`
	// Tokens are the theme's design-token overrides (token name → CSS value). The
	// store sanitizes them to the known allowlist; arbitrary CSS is not accepted.
	Tokens         map[string]string `json:"tokens"`
	BoardFlourish  string            `json:"board_flourish"`
	NumberFlourish string            `json:"number_flourish"`
}

// handleStyleCreate creates a theme.
//
//	Endpoint:  POST /api/styles
//	Auth:      permission:system-themes
//	Request:   {"name": "...", "tokens": {...}, "board_flourish": "...", "number_flourish": "..."}
//	Response:  201 StyleCreateResponse
func (s *Server) handleStyleCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permSystemThemes) {
		return
	}
	req, err := readJSON[styleWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Style name is required")
		return
	}
	id, err := s.store.CreateStyle(name, req.Tokens, req.BoardFlourish, req.NumberFlourish)
	if err != nil {
		writeInternalError(w, "create style", err)
		return
	}
	writeJSON(w, http.StatusCreated, model.StyleCreateResponse{ID: id, Name: name})
}

// handleStyleUpdate replaces a theme's fields. If the edited theme is the active
// one, the regenerated CSS is broadcast to every client so the live look updates.
//
//	Endpoint:    PUT /api/styles/{id}
//	Auth:        permission:system-themes
//	Request:     {"name": "...", "tokens": {...}, "board_flourish": "...", "number_flourish": "..."}
//	Response:    200 OKResponse
//	Broadcasts:  style_update (when the active style is edited)
func (s *Server) handleStyleUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permSystemThemes) {
		return
	}
	id, ok := pathInt64(w, r, "id", "style")
	if !ok {
		return
	}
	req, err := readJSON[styleWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Style name is required")
		return
	}
	if err := s.store.UpdateStyle(id, name, req.Tokens, req.BoardFlourish, req.NumberFlourish); err != nil {
		writeInternalError(w, "update style", err)
		return
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})

	// If this is the active style, broadcast the regenerated CSS to all clients.
	activeID, _ := s.store.GetSetting("active_style_id")
	if activeID == fmt.Sprintf("%d", id) {
		s.broadcastStyleUpdate(store.TokensToCSS(req.Tokens), req.BoardFlourish, req.NumberFlourish)
	}
}

// handleStyleDelete deletes a theme. If it was the active theme, the active
// setting is cleared and an empty style broadcast reverts every client to the
// default look.
//
//	Endpoint:    DELETE /api/styles/{id}
//	Auth:        permission:system-themes
//	Response:    204 No Content
//	Broadcasts:  style_update (when the active style is deleted)
func (s *Server) handleStyleDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permSystemThemes) {
		return
	}
	id, ok := pathInt64(w, r, "id", "style")
	if !ok {
		return
	}
	// If deleting the active style, clear the active setting + revert clients.
	activeID, _ := s.store.GetSetting("active_style_id")
	if activeID == fmt.Sprintf("%d", id) {
		_ = s.store.SetSetting("active_style_id", "")
		s.broadcastStyleUpdate("", "", "")
	}
	if _, err := s.store.DeleteStyle(id); err != nil {
		writeInternalError(w, "delete style", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleStyleActivate makes a style the active theme and broadcasts its CSS.
//
//	Endpoint:    POST /api/styles/{id}/activate
//	Auth:        permission:system-themes
//	Response:    200 OKResponse
//	Broadcasts:  style_update
func (s *Server) handleStyleActivate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permSystemThemes) {
		return
	}
	id, ok := pathInt64(w, r, "id", "style")
	if !ok {
		return
	}
	if err := s.store.SetSetting("active_style_id", fmt.Sprintf("%d", id)); err != nil {
		writeInternalError(w, "set active style", err)
		return
	}
	css, board, number := "", "", ""
	if active, _ := s.store.GetActiveStyle(); active != nil {
		css, board, number = active.CSSContent, active.BoardFlourish, active.NumberFlourish
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
	s.broadcastStyleUpdate(css, board, number)
}

// handleStyleDeactivate clears the active theme and reverts every client to the
// default look.
//
//	Endpoint:    POST /api/styles/deactivate
//	Auth:        permission:system-themes
//	Response:    200 OKResponse
//	Broadcasts:  style_update (empty)
func (s *Server) handleStyleDeactivate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permSystemThemes) {
		return
	}
	if err := s.store.SetSetting("active_style_id", ""); err != nil {
		writeInternalError(w, "clear active style", err)
		return
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
	s.broadcastStyleUpdate("", "", "")
}

// handleActiveStyleCSS returns the CSS of the currently active style.
//
//	Endpoint:  GET /api/styles/active
//	Auth:      public
//	Response:  {"css": "..."}
func (s *Server) handleActiveStyleCSS(w http.ResponseWriter, r *http.Request) {
	active, err := s.store.GetActiveStyle()
	if err != nil {
		writeInternalError(w, "get active style", err)
		return
	}
	css, board, number := "", "", ""
	if active != nil {
		css, board, number = active.CSSContent, active.BoardFlourish, active.NumberFlourish
	}
	writeJSON(w, http.StatusOK, model.ActiveCSSResponse{
		CSS:            css,
		BoardFlourish:  board,
		NumberFlourish: number,
	})
}
