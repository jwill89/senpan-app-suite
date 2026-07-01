package model

// Garapon is a festival lottery-drum event (ガラポン / 福引): a hand-crank drum a
// player spins to drop a colored ball and win a prize. Like a raffle it is created
// instantly "open" and later "closed" (archived), but players don't buy tickets —
// an admin issues each player a private tokenized link (a GaraponPlayer) with a
// fixed number of draws.
type Garapon struct {
	ID              int64  `json:"id"`
	Title           string `json:"title"`
	Details         string `json:"details"`           // markdown event description
	GrandPrizeImage string `json:"grand_prize_image"` // root-relative web path (images/garapons/…), "" = none
	Status          string `json:"status"`            // "open" or "closed"
	// Optional link to a Stamp Rally: when set, every drawing link issued for this
	// garapon also issues that participant a Stamp Rally card (sharing the token).
	StampRallyID    *int64 `json:"stamp_rally_id"`
	StampRallyTitle string `json:"stamp_rally_title,omitempty"` // read-only, joined for display
	CreatedAt       string `json:"created_at"`

	// Prizes is populated on detail fetches (the ball/prize tiers). Omitted from
	// list responses for efficiency.
	Prizes []GaraponPrize `json:"prizes,omitempty"`

	// Read-only aggregates populated for the admin list view only: how many
	// drawing links exist and how many draws have been made across them.
	PlayerCount int `json:"player_count,omitempty"`
	DrawCount   int `json:"draw_count,omitempty"`
}

// GaraponPrize is one prize tier in a garapon: a named prize, the color of the
// ball that wins it, and an appearance rate (a relative weight — the rates need
// not total 100; the server draws weighted by them). Exactly one prize per garapon
// is flagged IsGrand: the headline prize that carries the garapon's picture.
type GaraponPrize struct {
	ID        int64   `json:"id"`
	GaraponID int64   `json:"garapon_id"`
	Name      string  `json:"name"`
	BallColor string  `json:"ball_color"` // CSS color, e.g. "#e5b53f"
	Rate      float64 `json:"rate"`       // appearance weight (relative; normalized for display)
	IsGrand   bool    `json:"is_grand"`   // the grand (headline) prize
	SortOrder int     `json:"sort_order"`
}

// GaraponPlayer is a tokenized drawing link issued to a named player: an
// unguessable URL token, the player's name, and how many draws they're allowed.
// DrawsUsed is a read-only count of how many draws they've already made.
type GaraponPlayer struct {
	ID         int64  `json:"id"`
	GaraponID  int64  `json:"garapon_id"`
	Token      string `json:"token"` // unguessable URL token (the player's private link)
	PlayerName string `json:"player_name"`
	MaxDraws   int    `json:"max_draws"`
	DrawsUsed  int    `json:"draws_used"` // read-only: COUNT of recorded draws
	CreatedAt  string `json:"created_at"`
	// StampCardToken is the token of the Stamp Rally card auto-issued alongside this
	// drawing link when its garapon is linked to a rally — the SAME value as Token, so
	// one hash serves both /garapon/<token> and /stamp-card/<token>. "" when unlinked.
	StampCardToken string `json:"stamp_card_token,omitempty"`
}

// GaraponDraw is a single recorded pull: which player drew, the prize they won,
// and a snapshot of its name + ball color (so the log survives later prize edits).
type GaraponDraw struct {
	ID         int64  `json:"id"`
	GaraponID  int64  `json:"garapon_id"`
	PlayerID   int64  `json:"player_id"`
	PrizeID    int64  `json:"prize_id"`
	PlayerName string `json:"player_name"` // snapshot for the admin log
	PrizeName  string `json:"prize_name"`  // snapshot
	BallColor  string `json:"ball_color"`  // snapshot
	DrawnAt    string `json:"drawn_at"`
}

// GaraponsResponse is the body of GET /api/garapons — the admin garapon list
// (each carrying player_count/draw_count aggregates).
type GaraponsResponse struct {
	Garapons []Garapon `json:"garapons"`
}

// GaraponResponse wraps a single garapon — the body of POST /api/garapons
// {action:"create"} (HTTP 201) with the freshly created garapon.
type GaraponResponse struct {
	Garapon Garapon `json:"garapon"`
}

// GaraponDetailResponse is the body of GET /api/garapons/{id} — the admin detail:
// the garapon (with prizes), its drawing links, and the full draw log.
type GaraponDetailResponse struct {
	Garapon Garapon         `json:"garapon"`
	Players []GaraponPlayer `json:"players"`
	Draws   []GaraponDraw   `json:"draws"`
}

// GaraponPlayerResponse wraps a single drawing link — the body of POST
// /api/garapons/{id}/players {action:"create_player"} (HTTP 201).
type GaraponPlayerResponse struct {
	Player GaraponPlayer `json:"player"`
}

// PublicGarapon is the trimmed garapon shape sent to players: prizes carry names +
// ball colors + which is grand, but the appearance rates are zeroed (odds stay
// admin-only). It is the exported equivalent of server.publicGarapon and is the
// FAITHFUL wire shape for GaraponPublicResponse.garapon.
//
// NOTE (wire-vs-api.ts discrepancy): api.ts types this response's `garapon` as the
// full `Garapon`, but the handler actually emits this trimmed struct (no
// stamp_rally_id / stamp_rally_title / created_at / aggregates, and zeroed rates).
// This struct matches the ACTUAL bytes.
type PublicGarapon struct {
	ID              int64          `json:"id"`
	Title           string         `json:"title"`
	Details         string         `json:"details"`
	GrandPrizeImage string         `json:"grand_prize_image"`
	Status          string         `json:"status"`
	Prizes          []GaraponPrize `json:"prizes"`
}

// GaraponPublicPlayer is the trimmed player shape sent to the public view (no
// token — the caller already holds it in the URL). Exported equivalent of
// server.publicPlayer.
type GaraponPublicPlayer struct {
	PlayerName string `json:"player_name"`
	MaxDraws   int    `json:"max_draws"`
	DrawsUsed  int    `json:"draws_used"`
}

// GaraponPublicResponse is the body of GET /api/garapon/{token} — the player-facing
// view: the garapon (no odds), the player's name + draw allowance/usage, and their
// own draw history.
type GaraponPublicResponse struct {
	Garapon PublicGarapon       `json:"garapon"`
	Player  GaraponPublicPlayer `json:"player"`
	Draws   []GaraponDraw       `json:"draws"`
}

// GaraponDrawResponse is the body of POST /api/garapon/{token}/draw — the recorded
// draw plus the fresh usage counts.
type GaraponDrawResponse struct {
	Draw      GaraponDraw `json:"draw"`
	DrawsUsed int         `json:"draws_used"`
	MaxDraws  int         `json:"max_draws"`
}
