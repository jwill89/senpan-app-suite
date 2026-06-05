package server

import (
	"fmt"
	"net/http"
	"strings"
)

// styleRequest is the JSON body for POST /api/styles.
// Action determines the operation: "get", "create", "update", "delete", or "set_active".
type styleRequest struct {
	Action     string `json:"action"`
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CSSContent string `json:"css_content"`
}

// handleStylesList returns all styles (without CSS content) and the active style ID.
//
//	Endpoint:  GET /api/styles
//	Auth:      admin
//	Response:  {"styles": [...], "active_style_id": "..."}
func (s *Server) handleStylesList(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	styles, err := s.store.ListStyles()
	if err != nil {
		writeInternalError(w, "list styles", err)
		return
	}
	activeID, _ := s.store.GetSetting("active_style_id")
	writeJSON(w, http.StatusOK, map[string]any{
		"styles":          styles,
		"active_style_id": activeID,
	})
}

// handleStylesAction processes style CRUD and activation operations.
//
//	Endpoint:    POST /api/styles
//	Auth:        admin
//	Request:     {"action": "get"|"create"|"update"|"delete"|"set_active", ...}
//	Response:    varies by action
//	Broadcasts:  style_update (on update of active style, set_active, or delete of active)
func (s *Server) handleStylesAction(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	req, _ := readJSON[styleRequest](r)

	switch req.Action {
	case "get":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Style id is required")
			return
		}
		style, err := s.store.GetStyle(req.ID)
		if err != nil {
			writeInternalError(w, "get style", err)
			return
		}
		if style == nil {
			writeError(w, http.StatusNotFound, "Style not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"style": style})

	case "create":
		name := strings.TrimSpace(req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "Style name is required")
			return
		}
		id, err := s.store.CreateStyle(name, req.CSSContent)
		if err != nil {
			writeInternalError(w, "create style", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"id":   id,
			"name": name,
		})

	case "update":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Style id is required")
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "Style name is required")
			return
		}
		if err := s.store.UpdateStyle(req.ID, name, req.CSSContent); err != nil {
			writeInternalError(w, "update style", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

		// If this is the active style, broadcast the update to all clients
		activeID, _ := s.store.GetSetting("active_style_id")
		if activeID == fmt.Sprintf("%d", req.ID) {
			s.broadcastStyleUpdate(req.CSSContent)
		}

	case "delete":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Style id is required")
			return
		}
		// If deleting the active style, clear the active setting
		activeID, _ := s.store.GetSetting("active_style_id")
		if activeID == fmt.Sprintf("%d", req.ID) {
			_ = s.store.SetSetting("active_style_id", "")
			s.broadcastStyleUpdate("")
		}
		deleted, err := s.store.DeleteStyle(req.ID)
		if err != nil {
			writeInternalError(w, "delete style", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})

	case "set_active":
		idStr := ""
		if req.ID > 0 {
			idStr = fmt.Sprintf("%d", req.ID)
		}
		if err := s.store.SetSetting("active_style_id", idStr); err != nil {
			writeInternalError(w, "set active style", err)
			return
		}
		css := ""
		if req.ID > 0 {
			css, _ = s.store.GetActiveStyleCSS()
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		s.broadcastStyleUpdate(css)

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: get, create, update, delete, set_active")
	}
}

// handleActiveStyleCSS returns the CSS of the currently active style.
//
//	Endpoint:  GET /api/styles/active
//	Auth:      public
//	Response:  {"css": "..."}
func (s *Server) handleActiveStyleCSS(w http.ResponseWriter, r *http.Request) {
	css, err := s.store.GetActiveStyleCSS()
	if err != nil {
		writeInternalError(w, "get active style CSS", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"css": css})
}
