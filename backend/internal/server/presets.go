package server

import (
	"net/http"
	"strings"

	"app-suite/internal/model"
)

// presetWriteRequest is the JSON body for creating (POST /api/presets) or
// replacing (PUT /api/presets/{id}) a game preset. The id comes from the path.
type presetWriteRequest struct {
	Name        string  `json:"name"`
	PatternIDs  []int64 `json:"pattern_ids"`
	GameDetails string  `json:"game_details"`
}

// validate checks the shared create/update requirements, writing a 400 and
// returning false when they aren't met.
func (req presetWriteRequest) validate(w http.ResponseWriter) (string, bool) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "Preset name is required")
		return "", false
	}
	if len(req.PatternIDs) == 0 {
		writeError(w, http.StatusBadRequest, "Select at least one win pattern")
		return "", false
	}
	return name, true
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
	writeJSON(w, http.StatusOK, model.PresetsResponse{Presets: presets})
}

// handlePresetCreate creates a game preset.
//
//	Endpoint:  POST /api/presets
//	Auth:      permission:bingo-presets
//	Response:  201 {"id": N}
func (s *Server) handlePresetCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoPresets) {
		return
	}
	req, err := readJSON[presetWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	name, ok := req.validate(w)
	if !ok {
		return
	}
	id, err := s.store.CreateGamePreset(name, req.PatternIDs, req.GameDetails)
	if err != nil {
		writeInternalError(w, "presets", err)
		return
	}
	writeJSON(w, http.StatusCreated, model.PresetCreateResponse{ID: id})
}

// handlePresetUpdate replaces a game preset.
//
//	Endpoint:  PUT /api/presets/{id}
//	Auth:      permission:bingo-presets
//	Response:  200 {"ok": true}
func (s *Server) handlePresetUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoPresets) {
		return
	}
	id, ok := pathInt64(w, r, "id", "preset")
	if !ok {
		return
	}
	req, err := readJSON[presetWriteRequest](w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}
	name, ok := req.validate(w)
	if !ok {
		return
	}
	if err := s.store.UpdateGamePreset(id, name, req.PatternIDs, req.GameDetails); err != nil {
		writeInternalError(w, "presets", err)
		return
	}
	writeJSON(w, http.StatusOK, model.OKResponse{OK: true})
}

// handlePresetDelete deletes a game preset.
//
//	Endpoint:  DELETE /api/presets/{id}
//	Auth:      permission:bingo-presets
//	Response:  204 No Content
func (s *Server) handlePresetDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permBingoPresets) {
		return
	}
	id, ok := pathInt64(w, r, "id", "preset")
	if !ok {
		return
	}
	if _, err := s.store.DeleteGamePreset(id); err != nil {
		writeInternalError(w, "presets", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
