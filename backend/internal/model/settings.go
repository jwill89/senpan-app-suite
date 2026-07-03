package model

// UploadedFont is one uploaded font (group) in the public settings payload:
// its base name, its effective CSS family name (admin-customizable; defaults
// to the base name), and the current serving token of its publicly-served
// variant, which the frontend uses to build the @font-face URL
// (/api/fonts/pub/f/{token}). Tokens rotate on a schedule — always use the
// freshly served value.
type UploadedFont struct {
	Name   string `json:"name"`
	Family string `json:"family"`
	Token  string `json:"token"`
}

// SettingsResponse is the body of GET /api/settings. The settings map is keyed by
// setting name (a genuinely dynamic string→string map); uploaded_fonts lists the
// fonts available for the header/board font picker with their serving tokens.
type SettingsResponse struct {
	Settings      map[string]string `json:"settings"`
	UploadedFonts []UploadedFont    `json:"uploaded_fonts"`
}
