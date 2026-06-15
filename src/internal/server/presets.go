package server

import (
	"net/http"
	"strings"
)

// presetRequest is the JSON body for POST /api/presets.
// Action determines the operation: "create", "update", or "delete".
type presetRequest struct {
	Action      string  `json:"action"`
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	PatternIDs  []int64 `json:"pattern_ids"`
	GameDetails string  `json:"game_details"`
}

// handlePresetsList returns all saved game presets.
//
//	Endpoint:  GET /api/presets
//	Auth:      admin, or a user granted this page's permission
//	Response:  {"presets": [...]}
func (s *Server) handlePresetsList(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoPresets) {
		return
	}

	presets, err := s.store.ListGamePresets()
	if err != nil {
		writeInternalError(w, "presets", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"presets": presets})
}

// handlePresetsAction processes game-preset CRUD operations.
//
//	Endpoint:    POST /api/presets
//	Auth:        admin, or a user granted this page's permission
//	Request:     {"action": "create"|"update"|"delete", ...}
//	Response:    varies by action
func (s *Server) handlePresetsAction(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoPresets) {
		return
	}

	req, err := readJSON[presetRequest](r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	switch req.Action {
	case "create":
		name := strings.TrimSpace(req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "Preset name is required")
			return
		}
		if len(req.PatternIDs) == 0 {
			writeError(w, http.StatusBadRequest, "Select at least one win pattern")
			return
		}
		id, err := s.store.CreateGamePreset(name, req.PatternIDs, req.GameDetails)
		if err != nil {
			writeInternalError(w, "presets", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})

	case "update":
		name := strings.TrimSpace(req.Name)
		if req.ID <= 0 || name == "" {
			writeError(w, http.StatusBadRequest, "Provide preset id and name")
			return
		}
		if len(req.PatternIDs) == 0 {
			writeError(w, http.StatusBadRequest, "Select at least one win pattern")
			return
		}
		if err := s.store.UpdateGamePreset(req.ID, name, req.PatternIDs, req.GameDetails); err != nil {
			writeInternalError(w, "presets", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})

	case "delete":
		if req.ID <= 0 {
			writeError(w, http.StatusBadRequest, "Preset id is required")
			return
		}
		deleted, err := s.store.DeleteGamePreset(req.ID)
		if err != nil {
			writeInternalError(w, "presets", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})

	default:
		writeError(w, http.StatusBadRequest, "Invalid action. Use: create, update, delete")
	}
}
