package model

// Style represents a custom CSS theme stored in the database.
// Admins can create, edit, and activate styles to customize the player UI.
type Style struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Tokens is the theme's design-token overrides: a map of token name (without
	// the leading "--", e.g. "page-bg") to its CSS value. This is the source of
	// truth for a theme — the applied stylesheet is generated from it (see
	// store.TokensToCSS). Only known token names are stored; arbitrary CSS is not
	// supported, which keeps themes safe and lets the base stylesheet be
	// refactored freely. Populated on detail/active reads.
	Tokens map[string]string `json:"tokens,omitempty"`
	// CSSContent is the generated ":root{…}" stylesheet for this theme, derived
	// from Tokens. It is never stored — it's filled in on read for the active-CSS
	// endpoint and the live style broadcast — so it's omitted when empty.
	CSSContent string `json:"css_content,omitempty"`
	// Optional per-theme decorative flourishes (root-relative paths into
	// images/flourishes, "" = use the app's built-in art). BoardFlourish is the
	// SVG drawn at the player board corners; NumberFlourish flanks the last-called
	// number (player + admin Game tab).
	BoardFlourish  string `json:"board_flourish"`
	NumberFlourish string `json:"number_flourish"`
	CreatedAt      string `json:"created_at"`
}

// StylesResponse is the body of GET /api/styles: all styles (without CSS) and
// the active style id (a stringified id, or "").
type StylesResponse struct {
	Styles        []Style `json:"styles"`
	ActiveStyleID string  `json:"active_style_id"`
}

// StyleGetResponse is the body of POST /api/styles {action:"get"}: one style.
type StyleGetResponse struct {
	Style Style `json:"style"`
}

// StyleCreateResponse is the body of POST /api/styles {action:"create"}: the new
// style's id and name. (api.ts's StyleCreateResponse lists only `id`; the handler
// also returns `name`, which is authoritative and kept here.)
type StyleCreateResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ActiveCSSResponse is the body of GET /api/styles/active: the active theme's raw
// CSS and decorative flourish paths.
type ActiveCSSResponse struct {
	CSS            string `json:"css"`
	BoardFlourish  string `json:"board_flourish"`
	NumberFlourish string `json:"number_flourish"`
}
