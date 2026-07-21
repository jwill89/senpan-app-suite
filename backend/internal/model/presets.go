package model

// GamePreset is a reusable bingo-game template: a named set of win pattern IDs
// plus pre-written game details (markdown). Admins select a preset when
// starting a new game to auto-apply its patterns and details.
type GamePreset struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	PatternIDs  []int64 `json:"pattern_ids"`  // win pattern IDs to pre-select
	GameDetails string  `json:"game_details"` // markdown game details to apply
	// Auto pre-selects the automatic-draw toggle when the preset is applied, and
	// AutoInterval fills the "Time Between Calls" selector (seconds). A game
	// started from the preset begins auto-drawing when Auto is set; adjusting the
	// live game's auto controls never writes back to the preset.
	Auto         bool `json:"auto"`
	AutoInterval int  `json:"auto_interval"`
	CreatedAt    string `json:"created_at"`
}

// PresetsResponse is the body of GET /api/presets: all saved game presets.
type PresetsResponse struct {
	Presets []GamePreset `json:"presets"`
}

// PresetCreateResponse is the body of POST /api/presets {action:"create"}: the
// new preset's id.
type PresetCreateResponse struct {
	ID int64 `json:"id"`
}
