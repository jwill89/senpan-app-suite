// Package settings names the site settings kept in the settings table, each as text
// under its key, and the value each takes when it has never been saved.
package settings

import (
	"strconv"

	"app-suite/internal/bookclubs"
)

// The settings' keys.
const (
	AppTitle                = "app_title"
	DefaultDrawDelay        = "default_draw_delay"
	FrequentWinnerThreshold = "frequent_winner_threshold"
	FrequentWinnerHours     = "frequent_winner_hours"
	HeaderFont              = "header_font"
	GoogleFontsAPIKey       = "google_fonts_api_key" //nolint:gosec // the setting's name, not the key
	AniListAPIURL           = "anilist_api_url"
	BingoJoinPrompt         = "bingo_join_prompt"
	YoeverCooldownSeconds   = "yoever_cooldown_seconds"
	CustomCardCost          = "custom_card_cost"
	HideBingo               = "hide_bingo"
	HideCustomCards         = "hide_custom_cards"
	// TeaRoomOrigins is the sites whose pages may read the public tea-room API from
	// a browser: a JSON array of origins (internal/origins). It has no default, so
	// saved empty it stays empty and no cross-origin read is allowed.
	TeaRoomOrigins = "tea_room_origins"
	// UpdatedAt is when the site settings were last saved, RFC 3339: a save made
	// from an older copy is refused (409).
	UpdatedAt = "settings_updated_at"
)

// DefaultYoeverCooldown is the seconds a card waits between Yoever reactions.
const DefaultYoeverCooldown = 180

// Defaults is what a setting is when it has never been saved, or is saved empty.
var Defaults = map[string]string{
	AppTitle:                "Senpan App Suite",
	DefaultDrawDelay:        "0",
	FrequentWinnerThreshold: "3",
	FrequentWinnerHours:     "12",
	HeaderFont:              "Arapey",
	AniListAPIURL:           bookclubs.DefaultAniListURL,
	BingoJoinPrompt:         "Enter your unique bingo board ID to play",
	YoeverCooldownSeconds:   strconv.Itoa(DefaultYoeverCooldown),
	CustomCardCost:          "0",
	HideBingo:               "0",
	HideCustomCards:         "0",
}
