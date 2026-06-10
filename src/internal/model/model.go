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
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CSSContent string `json:"css_content,omitempty"` // omitted in list responses for efficiency
	CreatedAt  string `json:"created_at"`
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
	AvailableFrom      string  `json:"available_from"` // ISO datetime string; empty = always open
	AvailableTo        string  `json:"available_to"`   // ISO datetime string; empty = no end
	PrizeImage         string  `json:"prize_image"`    // relative web path to uploaded image
	Status             string  `json:"status"`         // "open" or "closed"
	WinnerEntryID      *int64  `json:"winner_entry_id"`
	CreatedAt          string  `json:"created_at"`
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

// BookClubEvent is a scheduled event post for a book club (e.g. a monthly
// meeting or watch party). It is rendered as a Discord embed and posted
// automatically to the club's events-channel webhook at PostAtUnix.
//
// The admin enters wall-clock times (StartLocal/PostAtLocal) together with
// their IANA Timezone, so the form can be edited later in the original local
// time. The absolute instants (StartAtUnix/PostAtUnix, UTC seconds) are
// computed server-side from those and drive scheduling plus Discord's
// timezone-aware <t:…> timestamps (which each viewer sees in their own zone).
type BookClubEvent struct {
	ID          int64  `json:"id"`
	ClubSlug    string `json:"club_slug"`
	Title       string `json:"title"`
	StartLocal  string `json:"start_local"`   // "2006-01-02T15:04" in Timezone
	Timezone    string `json:"timezone"`      // IANA name, e.g. "America/New_York"
	LengthHours int    `json:"length_hours"`  // meeting length in hours (1–5)
	Location    string `json:"location"`      // free text (e.g. a Discord voice channel)
	Image       string `json:"image"`         // full URL, shown full-width in the embed
	PostAtLocal string `json:"post_at_local"` // "2006-01-02T15:04" in Timezone
	StartAtUnix int64  `json:"start_at_unix"` // computed UTC seconds
	PostAtUnix  int64  `json:"post_at_unix"`  // computed UTC seconds
	Posted      bool   `json:"posted"`        // whether it has been posted to Discord
	PostedAt    string `json:"posted_at"`     // ISO timestamp it was posted (empty if not)
	CreatedAt   string `json:"created_at"`
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
