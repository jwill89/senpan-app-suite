package model

// UploadedFont is one uploaded font (group) as the settings list it: its base
// name, its effective CSS family name (admin-customizable; defaults to the base
// name), and the current serving token of its publicly-served variant, which the
// frontend builds the @font-face URL from. Tokens rotate on a schedule - always
// use the freshly served value.
type UploadedFont struct {
	Name   string `json:"name"`
	Family string `json:"family"`
	Token  string `json:"token"`
}
