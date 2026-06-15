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
	"anilist_api_url",
	"bingo_join_prompt",
}

// settingsDefaults provides fallback values for settings that have not been configured.
var settingsDefaults = map[string]string{
	"app_title":                 "Nifty App Suite",
	"default_draw_delay":        "0",
	"frequent_winner_threshold": "3",
	"frequent_winner_hours":     "12",
	"header_font":               "Arapey",
	"anilist_api_url":           defaultAniListURL,
	"bingo_join_prompt":         "Enter your unique bingo board ID to play",
}

// secretSettings are setting keys that must not be exposed to non-admin
// callers. GET /api/settings is public, so these are blanked out unless the
// requester is an authenticated admin. Per-club Discord webhook URLs grant
// write access to a channel and are registered here by bookclubs.go's init().
var secretSettings = map[string]bool{}

// handleSettingsGet returns all app settings as a key-value map.
//
//	Endpoint:  GET /api/settings
//	Auth:      public (settings are not sensitive)
//	Response:  {"settings": {"app_title": "...", ...}, "uploaded_fonts": ["My Font.ttf", ...]}
func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	admin := s.isAdmin(r)
	result := make(map[string]string, len(settingsKeys))
	for _, key := range settingsKeys {
		// Never leak secret settings (e.g. the Discord webhook URL) to public
		// callers — only admins, who need them to edit on the settings page.
		if secretSettings[key] && !admin {
			result[key] = ""
			continue
		}
		val, err := s.store.GetSetting(key)
		if err != nil || val == "" {
			val = settingsDefaults[key]
		}
		result[key] = val
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": result,
		// Uploaded font filenames so the frontend can register @font-face rules
		// and offer them in the header-font picker (alongside Google Fonts).
		"uploaded_fonts": s.fontFileNames(),
	})
}

// settingsRequest is the JSON body for POST /api/settings.
type settingsRequest struct {
	Settings map[string]string `json:"settings"`
}

// handleSettingsUpdate saves one or more app settings.
// Validates numeric settings and broadcasts changes via WebSocket.
//
//	Endpoint:    POST /api/settings
//	Auth:        admin, or a user granted this page's permission
//	Request:     {"settings": {"app_title": "...", "default_draw_delay": "5", ...}}
//	Response:    {"ok": true}
//	Broadcasts:  settings_update (when app_title or header_font changes)
func (s *Server) handleSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, permSystemSettings) {
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
		Type          string   `json:"type"`
		Title         string   `json:"app_title,omitempty"`
		HeaderFont    string   `json:"header_font,omitempty"`
		UploadedFonts []string `json:"uploaded_fonts,omitempty"`
	}{Type: "settings_update"}

	if v, ok := req.Settings["app_title"]; ok {
		broadcastPayload.Title = v
	}
	if v, ok := req.Settings["header_font"]; ok {
		broadcastPayload.HeaderFont = v
		// Include the current uploaded fonts so every client can register the
		// @font-face for a newly selected uploaded font without a reload.
		broadcastPayload.UploadedFonts = s.fontFileNames()
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
