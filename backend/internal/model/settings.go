package model

// SettingsResponse is the body of GET /api/settings. The settings map is keyed by
// setting name (a genuinely dynamic string→string map); uploaded_fonts lists the
// font filenames available for the header/board font picker.
type SettingsResponse struct {
	Settings      map[string]string `json:"settings"`
	UploadedFonts []string          `json:"uploaded_fonts"`
}
