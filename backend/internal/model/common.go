package model

// OKResponse is the minimal success envelope for actions whose result is just
// "it worked" — JSON: {"ok": true}.
type OKResponse struct {
	OK bool `json:"ok"`
}

// DeletedResponse reports whether a delete removed a row — JSON: {"deleted": true}.
type DeletedResponse struct {
	Deleted bool `json:"deleted"`
}

// DeletedCountResponse reports how many rows a bulk delete removed — {"deleted": N}.
type DeletedCountResponse struct {
	Deleted int64 `json:"deleted"`
}

// StatusResponse is the success envelope for actions that flip a status flag —
// JSON: {"ok": true, "status": "open"}. Shared by raffle/garapon/stamp-rally
// set_status (and similar) actions.
type StatusResponse struct {
	OK     bool   `json:"ok"`
	Status string `json:"status"`
}

// NamedOKResponse is a success envelope carrying a resulting name — JSON:
// {"ok": true, "name": "..."}. Used by the font rename and carrd create_dir
// actions.
type NamedOKResponse struct {
	OK   bool   `json:"ok"`
	Name string `json:"name"`
}

// RenamedResponse reports whether a rename changed a row — JSON: {"renamed": true}.
type RenamedResponse struct {
	Renamed bool `json:"renamed"`
}

// PausedResponse reports a success plus a stamp's new paused flag — JSON:
// {"ok": true, "paused": <bool>}.
type PausedResponse struct {
	OK     bool `json:"ok"`
	Paused bool `json:"paused"`
}

// SkippedUpload is one file a multi-file upload could not store, with the
// reason it was skipped. Shared by the font/carrd/image upload responses, which
// all return {"uploaded": [...], "skipped": [{name, reason}]}.
type SkippedUpload struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}
