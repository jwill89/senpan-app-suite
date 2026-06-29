// Package model defines the domain types shared across the application.
// These types carry no logic and are used as data-transfer objects between
// the store, service, and server layers. All struct tags are for JSON
// serialization to/from the API and database.
package model

// Card represents a bingo card stored in the database.
// BoardData is a 5×5 grid where board[row][col] holds the number;
// col 0=B(1–15), col 1=I(16–30), col 2=N(31–45), col 3=G(46–60), col 4=O(61–75).
// The centre cell [2][2] is always 0, representing the FREE space.
type Card struct {
	ID         string  `json:"id"`          // 6-char alphanumeric unique identifier
	BoardData  [][]int `json:"board_data"`  // 5×5 grid of numbers (0 = FREE)
	PlayerName string  `json:"player_name"` // optional player name assigned by admin
	Details    string  `json:"details"`     // optional extra info about the cardholder
	CreatedAt  string  `json:"created_at"`  // ISO timestamp the card was generated ("" if pre-dates tracking)
}

// User is an admin-area account. Authentication is username + password only;
// the argon2id password hash is never part of this struct (it stays in the
// store layer) so it can never leak through JSON serialization.
//
// Accounts are created inactive (via the hidden registration page) and remain
// so until an admin activates them. IsAdmin grants full access to every page;
// non-admin users are limited to the pages listed in Permissions (one key per
// admin page, mirroring the frontend AdminTab ids, e.g. "bingo-cards").
type User struct {
	ID          int64    `json:"id"`
	Username    string   `json:"username"`
	IsAdmin     bool     `json:"is_admin"`
	IsActive    bool     `json:"is_active"`
	Permissions []string `json:"permissions"` // page-permission keys (ignored when IsAdmin)
	CreatedAt   string   `json:"created_at"`
	LastLoginAt string   `json:"last_login_at"` // ISO timestamp of last successful login ("" if never)
}

// TokenInfo is the non-secret metadata about an account's personal access token
// (used by external API clients such as the FFXIV plugin). The token plaintext
// is never stored or re-shown, so only its prefix and timestamps are exposed.
// HasToken is false (and the other fields empty) when the account has no token.
type TokenInfo struct {
	HasToken   bool   `json:"has_token"`
	Prefix     string `json:"prefix"`       // leading visible chars, e.g. "pat_Ab12Cd34"
	CreatedAt  string `json:"created_at"`   // ISO timestamp ("" when none)
	LastUsedAt string `json:"last_used_at"` // ISO timestamp ("" when never used)
}

// PatternCategory groups win patterns into named, ordered categories
// for organizational display in the admin UI.
type PatternCategory struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"` // lower values appear first
}

// Pattern represents a reusable win pattern template.
// PatternData is a 5×5 boolean grid where true means the cell must be
// called (matched) for a card to win with this pattern.
type Pattern struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	PatternData  [][]bool `json:"pattern_data"`  // 5×5 grid; true = required cell
	SortOrder    int      `json:"sort_order"`    // display order within category
	CategoryID   int64    `json:"category_id"`   // FK to pattern_categories
	CategoryName string   `json:"category_name"` // denormalized for display
}

// BingoGamePattern is a point-in-time snapshot of a pattern attached to a bingo game.
// Snapshots are stored so that deleting or renaming a pattern does not
// affect games that were started with it.
type BingoGamePattern struct {
	ID          int      `json:"id"`           // original pattern ID at time of snapshot
	Name        string   `json:"name"`         // pattern name at time of snapshot
	PatternData [][]bool `json:"pattern_data"` // 5×5 boolean grid
}

// GamePreset is a reusable bingo-game template: a named set of win pattern IDs
// plus pre-written game details (markdown). Admins select a preset when
// starting a new game to auto-apply its patterns and details.
type GamePreset struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	PatternIDs  []int64 `json:"pattern_ids"`  // win pattern IDs to pre-select
	GameDetails string  `json:"game_details"` // markdown game details to apply
	CreatedAt   string  `json:"created_at"`
}

// BingoGame is the internal database representation of a bingo game row.
// It is not directly exposed to clients; use BingoGameState for the public view.
type BingoGame struct {
	ID           int64  // auto-increment primary key
	Status       string // "active" or "ended"
	CreatedAt    string // ISO 8601 timestamp
	WinnersCache string // raw JSON array of winning card IDs, computed on each draw
}

// BingoGameState is the public view of an active bingo game returned to clients.
// It contains everything a player or admin needs to render the current game.
type BingoGameState struct {
	ID            int64              `json:"id"`
	CreatedAt     string             `json:"created_at"`     // ISO 8601 timestamp the game started (UTC)
	CalledNumbers []int              `json:"called_numbers"` // numbers drawn so far, in call order
	Patterns      []BingoGamePattern `json:"patterns"`       // active win patterns for this game
	TotalCalled   int                `json:"total_called"`   // len(CalledNumbers), for convenience
}

// BingoDrawnNumber holds details of a single drawn bingo number, returned after each draw action.
type BingoDrawnNumber struct {
	Number    int    `json:"number"`     // the bingo number (1–75)
	Letter    string `json:"letter"`     // column letter: B, I, N, G, or O
	CallOrder int    `json:"call_order"` // 1-based position in the draw sequence
}

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

// Raffle represents a raffle event that players can enter.
// Raffles have configurable entry limits, costs, and time windows.
type Raffle struct {
	ID                 int64   `json:"id"`
	Title              string  `json:"title"`
	Description        string  `json:"description"`
	Rules              string  `json:"rules"`
	MaxEntries         int     `json:"max_entries"`         // max entries per player
	SignupInstructions string  `json:"signup_instructions"` // shown to player after signup
	CostPerEntry       float64 `json:"cost_per_entry"`
	AvailableFrom      string  `json:"available_from"` // UTC RFC-3339 datetime; empty = always open
	AvailableTo        string  `json:"available_to"`   // UTC RFC-3339 datetime; empty = no end
	PrizeImage         string  `json:"prize_image"`    // relative web path to uploaded image
	Status             string  `json:"status"`         // "open" or "closed"
	WinnerEntryID      *int64  `json:"winner_entry_id"`
	CreatedAt          string  `json:"created_at"`

	// Read-only aggregates populated for the admin list view only (the closed-raffle
	// table): the verified winner's "Character @ World", and the collected total
	// (sum of paid tickets × cost_per_entry). Omitted from public responses.
	WinnerName string  `json:"winner_name,omitempty"`
	PaidTotal  float64 `json:"paid_total,omitempty"`
}

// RaffleEntry represents a user's entry (one or more tickets) into a raffle.
type RaffleEntry struct {
	ID            int64  `json:"id"`
	RaffleID      int64  `json:"raffle_id"`
	CharacterName string `json:"character_name"` // in-game character name
	World         string `json:"world"`          // game world/server
	NumEntries    int    `json:"num_entries"`    // number of tickets purchased
	Paid          bool   `json:"paid"`           // admin-verified payment status
	CreatedAt     string `json:"created_at"`
}

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

// ReadingList is a named, ordered collection of book-club reading items
// (e.g. a "Yaoi Book Club" reading list). ClubSlug groups lists under a
// particular book club so additional clubs can be added later without a
// schema change. Items is populated only on detail fetches.
type ReadingList struct {
	ID        int64             `json:"id"`
	ClubSlug  string            `json:"club_slug"` // book club identifier, e.g. "yaoi"
	Title     string            `json:"title"`
	CreatedAt string            `json:"created_at"`
	Items     []ReadingListItem `json:"items,omitempty"`
}

// ReadingListItem is a single entry in a reading list (a manga/manhwa/danmei
// title). CoverImage is stored as a full URL — either an uploaded image served
// from this site or a cover URL pulled from AniList.
type ReadingListItem struct {
	ID         int64               `json:"id"`
	ListID     int64               `json:"list_id"`
	CoverImage string              `json:"cover_image"` // full URL
	Title      string              `json:"title"`
	Summary    string              `json:"summary"`  // markdown (rendered by Discord on publish)
	Format     string              `json:"format"`   // Manga, Manhwa, Danmei, etc.
	Genres     string              `json:"genres"`   // comma-separated list
	Tropes     string              `json:"tropes"`   // comma-separated list
	Chapters   string              `json:"chapters"` // free text (e.g. "156", "156 (ongoing)")
	Comments   string              `json:"comments"` // Yao's Comments (markdown)
	Sources    []ReadingListSource `json:"sources"`  // external links (stored as JSON)
	SortOrder  int                 `json:"sort_order"`
}

// ReadingListSource is a named external link attached to a reading list item.
type ReadingListSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// WinnersLogEntry represents a confirmed winner stored in the winners log.
// Created when an admin ends a game and selects valid winners.
type WinnersLogEntry struct {
	ID              int64  `json:"id"`
	LoggedAt        string `json:"logged_at"`        // ISO 8601 timestamp of when the win was logged
	CardID          string `json:"card_id"`          // winning card ID
	PlayerName      string `json:"player_name"`      // player name from the card at time of win
	GameDetails     string `json:"game_details"`     // game details setting at time of win
	WinningPatterns string `json:"winning_patterns"` // JSON array of pattern names active during the game
}

// FrequentWinner represents a player who has won multiple times within
// a recent time window. Used to alert admins of potential repeat winners.
type FrequentWinner struct {
	PlayerName string `json:"player_name"`
	WinCount   int    `json:"win_count"` // number of wins in the lookback period
}

// AnnouncementType is a named Discord destination for announcements: a friendly
// label plus the webhook URL of the channel its announcements post to. Each
// announcement references one type, and posts to that type's webhook.
type AnnouncementType struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	WebhookURL string `json:"webhook_url"` // Discord channel webhook
	CreatedAt  string `json:"created_at"`
}

// AnnouncementRole is a named Discord role that an announcement can optionally
// tag (ping) when it posts: a friendly group label plus the role's Discord ID.
// It's a convenience picker — managed like announcement types — so admins choose
// a role by name instead of pasting raw snowflake IDs. Tagging happens in the
// message content (above the embed), because role mentions inside an embed don't
// notify anyone.
type AnnouncementRole struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	RoleID    string `json:"role_id"` // Discord role snowflake ID (string to avoid JS precision loss)
	CreatedAt string `json:"created_at"`
}

// Announcement is an admin-authored message posted to Discord as an embed via
// its type's webhook — manually ("send now") or automatically on a schedule.
//
// One IANA Timezone anchors every time on the announcement: the admin enters
// wall-clock values (StartLocal/EndLocal for the event window, OnceLocal for a
// one-time post, and the recurring time-of-day/weekday selections) and the
// backend resolves them in Timezone with time.LoadLocation. The absolute instants
// (StartAt/EndAt/NextPostAt, UTC RFC-3339) are computed server-side and drive the
// embed timestamps + the scheduler. Because the zone is explicit, all times stay
// put across DST transitions (e.g. "every Saturday 7pm America/New_York").
type Announcement struct {
	ID         int64  `json:"id"`
	TypeID     int64  `json:"type_id"`
	Title      string `json:"title"`
	Details    string `json:"details"`     // markdown
	Image      string `json:"image"`       // full URL, shown full-width at the bottom of the embed
	Thumbnail  string `json:"thumbnail"`   // full URL, shown as the small top-right embed thumbnail
	Color      string `json:"color"`       // embed accent colour, "#rrggbb" ("" = brand default)
	Location   string `json:"location"`    // optional free-text location (e.g. a Discord voice channel)
	StartLocal string `json:"start_local"` // optional event start, wall-clock "2006-01-02T15:04" in Timezone
	EndLocal   string `json:"end_local"`   // optional event end, wall-clock in Timezone
	StartAt    string `json:"start_at"`    // computed event start, UTC RFC-3339
	EndAt      string `json:"end_at"`      // computed event end, UTC RFC-3339
	// Discord timestamp styles (t|T|d|D|f|F|R) for how the start/end render in the
	// embed; each is chosen independently ("" = default — F for start, t for end).
	StartFormat string `json:"start_format"`
	EndFormat   string `json:"end_format"`
	// DynamicDates re-anchors the start/end onto the day the announcement is posted
	// (preserving the time-of-day and how many days the end runs past the start),
	// so a recurring "day-of" event post always shows the current occurrence rather
	// than the first one. StartLocal/EndLocal define the template time-of-day.
	DynamicDates bool `json:"dynamic_dates"`

	// Schedule (all optional; ScheduleKind == "" means unscheduled — manual only).
	// All recurring/one-time times are wall-clock values anchored to Timezone.
	ScheduleKind        string `json:"schedule_kind"`          // ""|once|daily|weekly|monthly
	Timezone            string `json:"timezone"`               // IANA zone anchoring every time on the announcement
	OnceLocal           string `json:"once_local"`             // wall-clock "2006-01-02T15:04" in Timezone (one-time schedule)
	ScheduleMinutes     int    `json:"schedule_minutes"`       // local minutes-of-day in Timezone (recurring)
	ScheduleWeekdays    string `json:"schedule_weekdays"`      // CSV of local weekdays 0=Sun..6=Sat (weekly; first value reused for monthly)
	ScheduleWeekOfMonth int    `json:"schedule_week_of_month"` // 1..5 or -1=last (monthly)

	NextPostAt   string `json:"next_post_at"`   // next scheduled instant, UTC RFC-3339 ("" = none)
	SkipNext     bool   `json:"skip_next"`      // skip the next occurrence, then resume
	Active       bool   `json:"active"`         // whether the schedule is live
	LastPostedAt string `json:"last_posted_at"` // ISO timestamp of last post ("" if never)
	CreatedAt    string `json:"created_at"`

	// Optional Discord link buttons (up to 5) rendered as a single action row
	// below the embed. Persisted as a JSON array in the announcements.buttons column.
	Buttons []AnnouncementButton `json:"buttons"`

	// Optional role tag posted in the message content (above the embed), since
	// mentions inside an embed don't notify. One of: "" (don't tag), "everyone"
	// (@everyone), or "role:<announcement_role_id>" (a managed AnnouncementRole).
	Mention string `json:"mention"`

	// Read-only convenience for list rendering (joined from announcement_types).
	TypeName string `json:"type_name,omitempty"`
}

// AnnouncementButton is one Discord link button shown beneath an announcement's
// embed: a label, an optional emoji (a unicode emoji like "🎉" or a custom-emoji
// token "<:name:id>"/"<a:name:id>"), and the URL it opens.
type AnnouncementButton struct {
	Label string `json:"label"`
	Emoji string `json:"emoji"`
	URL   string `json:"url"`
}

// Affiliate is a partner establishment listed in the Senpan Tea House → Affiliates
// admin section: a named place with one or more owners, a location, opening hours
// (multiple time ranges sharing one timezone), markdown details, and two pictures
// picked from dedicated permanent image categories — a Logo and an establishment
// Screenshot. Owners and Hours are persisted as JSON columns (no sub-tables).
type Affiliate struct {
	ID         int64           `json:"id"`
	Name       string          `json:"name"`       // establishment name
	Owners     []string        `json:"owners"`     // one or more owner names
	Location   string          `json:"location"`   // free-text location
	Timezone   string          `json:"timezone"`   // IANA zone anchoring every Hours entry
	Hours      []AffiliateHour `json:"hours"`      // open time ranges (wall-clock in Timezone)
	Details    string          `json:"details"`    // markdown
	Logo       string          `json:"logo"`       // root-relative web path (images/affiliate_logos/…)
	Screenshot string          `json:"screenshot"` // root-relative web path (images/affiliate_images/…)
	CreatedAt  string          `json:"created_at"`
}

// AffiliateHour is one opening-hours entry on an affiliate: an optional label
// (e.g. "Mon–Fri" or "Weekends"), a required start time, and an optional end time.
// Times are wall-clock "HH:MM" values interpreted in the affiliate's Timezone.
type AffiliateHour struct {
	Label string `json:"label"` // optional descriptor, e.g. "Mon–Fri"
	Start string `json:"start"` // wall-clock "HH:MM"
	End   string `json:"end"`   // optional wall-clock "HH:MM"
}

// ── Stamp Rally ──────────────────────────────────────────────────────────────
//
// A Stamp Rally (Festival → Stamp Rally) is an event where participants visit
// affiliate stalls and enter a per-stall password on their personal digital stamp
// card to collect that stamp. Once every still-collectable stamp is collected the
// card completes and its prizes (with redemption instructions) are revealed. Like
// a Garapon, the admin authors the event (with its stamps + prizes) and issues each
// participant a tokenized card link; the token is the capability for the public view.

// Placement positions a stamp or prize on the card image: x/y/width/height are
// percentages of the card image's box (0–100) and Rotation is in degrees. The
// admin sets these by dragging/resizing/rotating in the visual placement editor.
type Placement struct {
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
	Rotation float64 `json:"rotation"`
}

// StampRally is the event: a named card with a background image, a default
// "not stamped" placeholder image, an availability window, markdown details and
// redeem instructions, plus its stamps and prizes (loaded on detail fetches only).
type StampRally struct {
	ID                 int64  `json:"id"`
	Title              string `json:"title"`
	CardImage          string `json:"card_image"`          // images/stamp_cards/… (background)
	NotStampedImage    string `json:"not_stamped_image"`   // images/stamp_stamps/… (uncollected/locked placeholder)
	AvailableFrom      string `json:"available_from"`      // UTC RFC-3339 ("" = unbounded)
	AvailableTo        string `json:"available_to"`        // UTC RFC-3339 ("" = unbounded)
	Details            string `json:"details"`             // markdown
	RedeemInstructions string `json:"redeem_instructions"` // markdown, shown once complete
	// Status is a manual "open"/"closed" flag, independent of the availability window:
	// a closed rally is read-only (no more stamping), moves to the admin's closed table,
	// and isn't offered for Garapon linking.
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`

	// Populated on detail fetches only (omitted from list responses for efficiency).
	Stamps []StampRallyStamp `json:"stamps,omitempty"`
	Prizes []StampRallyPrize `json:"prizes,omitempty"`

	// Read-only aggregates for the admin list view: issued cards, how many of them
	// have been completed, and the stall (stamp) counts used by the list's at-a-glance
	// "X/Y stalls active" summary (ActiveStampCount = stamps not paused). Omitted
	// (zero) on detail/public responses.
	CardCount        int `json:"card_count,omitempty"`
	CompletedCount   int `json:"completed_count,omitempty"`
	StampCount       int `json:"stamp_count,omitempty"`
	ActiveStampCount int `json:"active_stamp_count,omitempty"`
}

// StampRallyStamp is one collectable stamp on a rally card: an image, the password
// a participant enters to collect it, its placement on the card, an optional active
// window (within the event window) and a manual pause toggle, and the affiliate
// (stall) it belongs to. AffiliateID is nil for the "Senpan Tea House" default.
type StampRallyStamp struct {
	ID            int64  `json:"id"`
	RallyID       int64  `json:"rally_id"`
	AffiliateID   *int64 `json:"affiliate_id"`       // nil = Senpan Tea House (default)
	AffiliateName string `json:"affiliate_name"`     // joined for display ("" → "Senpan Tea House")
	Image         string `json:"image"`              // images/stamp_stamps/…
	Password      string `json:"password,omitempty"` // omitted from public payloads
	Placement     `json:"placement"`
	ActiveFrom    string `json:"active_from"` // UTC RFC-3339 within event window ("" = whole event)
	ActiveTo      string `json:"active_to"`   // UTC RFC-3339 ("" = whole event)
	Paused        bool   `json:"paused"`
	SortOrder     int    `json:"sort_order"`
}

// StampRallyPrize is a reward revealed once a card completes: a name, an image, and
// its placement on the card. Before completion the card shows the not-stamped
// placeholder at the prize's slot instead.
type StampRallyPrize struct {
	ID        int64  `json:"id"`
	RallyID   int64  `json:"rally_id"`
	Name      string `json:"name"`
	Image     string `json:"image"` // images/stamp_prizes/…
	Placement `json:"placement"`
	SortOrder int `json:"sort_order"`
}

// StampRallyCard is a participant's tokenized card for a rally: an unguessable URL
// token (the public capability), the participant's name, and completion state.
// CollectedCount is a read-only aggregate populated for admin listings.
type StampRallyCard struct {
	ID              int64  `json:"id"`
	RallyID         int64  `json:"rally_id"`
	Token           string `json:"token"`
	ParticipantName string `json:"participant_name"`
	Completed       bool   `json:"completed"`
	CompletedAt     string `json:"completed_at"`
	CreatedAt       string `json:"created_at"`
	CollectedCount  int    `json:"collected_count,omitempty"`
}

// StampRallyCollected records one stamp a participant collected on their card, with
// the time it was stamped. The (card_id, stamp_id) pair is unique — a stamp can't be
// collected twice and collection can't be undone. ParticipantName + StallName are
// snapshotted at collect time so the stamp log survives card/stamp deletion (RallyID
// cascade-deletes the row only when the whole rally is removed).
type StampRallyCollected struct {
	ID              int64  `json:"id"`
	RallyID         int64  `json:"rally_id"`
	CardID          int64  `json:"card_id"`
	StampID         int64  `json:"stamp_id"`
	ParticipantName string `json:"participant_name"`
	StallName       string `json:"stall_name"`
	StampedAt       string `json:"stamped_at"`
}

// StampRallyLogEntry is one row of the event-wide stamp log (the admin "View Logs"
// page): which participant collected which stall's stamp, and when. Rows are grouped
// by participant in the UI. StallName is the stamp's affiliate, or "Senpan Tea House".
type StampRallyLogEntry struct {
	CardID          int64  `json:"card_id"`
	ParticipantName string `json:"participant_name"`
	StampID         int64  `json:"stamp_id"`
	StallName       string `json:"stall_name"`
	StampedAt       string `json:"stamped_at"`
}
