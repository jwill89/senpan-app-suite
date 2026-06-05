package server

import (
	"net/http"
	"strconv"
)

// settingsKeys lists the setting keys exposed via the settings API.
var settingsKeys = []string{
	"app_title",
	"default_draw_delay",
	"frequent_winner_threshold",
	"frequent_winner_hours",
	"header_font",
	"google_fonts_api_key",
}

// settingsDefaults provides fallback values for settings that have not been configured.
var settingsDefaults = map[string]string{
	"app_title":                 "Nifty App Suite",
	"default_draw_delay":        "0",
	"frequent_winner_threshold": "3",
	"frequent_winner_hours":     "12",
	"header_font":               "Arapey",
}

// handleSettingsGet returns all app settings as a key-value map.
//
//	Endpoint:  GET /api/settings
//	Auth:      public (settings are not sensitive)
//	Response:  {"settings": {"app_title": "...", "default_draw_delay": "0", ...}}
func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	result := make(map[string]string, len(settingsKeys))
	for _, key := range settingsKeys {
		val, err := s.store.GetSetting(key)
		if err != nil || val == "" {
			val = settingsDefaults[key]
		}
		result[key] = val
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": result})
}

// settingsRequest is the JSON body for POST /api/settings.
type settingsRequest struct {
	Settings map[string]string `json:"settings"`
}

// handleSettingsUpdate saves one or more app settings.
// Validates numeric settings and broadcasts changes via WebSocket.
//
//	Endpoint:    POST /api/settings
//	Auth:        admin
//	Request:     {"settings": {"app_title": "...", "default_draw_delay": "5", ...}}
//	Response:    {"ok": true}
//	Broadcasts:  settings_update (when app_title or header_font changes)
func (s *Server) handleSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	req, err := readJSON[settingsRequest](r)
	if err != nil || req.Settings == nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Validate and save each setting
	for key, val := range req.Settings {
		if !isAllowedSetting(key) {
			writeError(w, http.StatusBadRequest, "Unknown setting: "+key)
			return
		}
		// Validate numeric settings
		switch key {
		case "default_draw_delay":
			n, err := strconv.Atoi(val)
			if err != nil || n < 0 || n > 60 {
				writeError(w, http.StatusBadRequest, "Draw delay must be 0–60")
				return
			}
		case "frequent_winner_threshold":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 || n > 100 {
				writeError(w, http.StatusBadRequest, "Winner threshold must be 1–100")
				return
			}
		case "frequent_winner_hours":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 || n > 168 {
				writeError(w, http.StatusBadRequest, "Winner hours must be 1–168")
				return
			}
		}
	}

	for key, val := range req.Settings {
		if err := s.store.SetSetting(key, val); err != nil {
			writeInternalError(w, "save setting", err)
			return
		}
	}

	// Broadcast changed settings to all clients
	broadcastPayload := struct {
		Type       string `json:"type"`
		Title      string `json:"app_title,omitempty"`
		HeaderFont string `json:"header_font,omitempty"`
	}{Type: "settings_update"}

	if v, ok := req.Settings["app_title"]; ok {
		broadcastPayload.Title = v
	}
	if v, ok := req.Settings["header_font"]; ok {
		broadcastPayload.HeaderFont = v
	}
	if broadcastPayload.Title != "" || broadcastPayload.HeaderFont != "" {
		s.hub.Broadcast(broadcastPayload)
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// isAllowedSetting checks whether a key is in the editable settings list.
func isAllowedSetting(key string) bool {
	for _, k := range settingsKeys {
		if k == key {
			return true
		}
	}
	return false
}

// getSettingInt reads a numeric setting with a default fallback.
func (s *Server) getSettingInt(key string, fallback int) int {
	val, err := s.store.GetSetting(key)
	if err != nil || val == "" {
		return fallback
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return n
}
