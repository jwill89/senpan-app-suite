package game

import "github.com/danielgtaylor/huma/v2"

// The game's shapes as the API answers them (internal/api, game.go and board.go)
// and as its live events carry them. They are the document's schemas, so every
// field says what it is.

// GameState is the game as a console or a board opening it sees it: the game
// running, if any, its winners, and what goes with it.
type GameState struct {
	Game        *Game    `json:"game" doc:"The game running; null when there is none."`
	Winners     []string `json:"winners" nullable:"false" doc:"The ids of the cards that have won the game so far, in the order they won. Staff with the Game page alone see them: anyone else gets an empty list, as only the staff's live events carry them."`
	GameDetails string   `json:"gameDetails" doc:"The game's details (Markdown), shown to players; empty for none."`
	DrawDelay   int      `json:"drawDelay" minimum:"0" maximum:"60" doc:"Seconds a drawn number waits before players see it (the settings' defaultDrawDelay): the automatic draws use it, and the Game page draws with it."`
}

// Game is a game running.
type Game struct {
	ID            int64         `json:"id" doc:"The game's id."`
	CreatedAt     *string       `json:"createdAt" nullable:"true" doc:"When it started, RFC 3339 in UTC."`
	CalledNumbers []int         `json:"calledNumbers" nullable:"false" doc:"The numbers called, in the order they were called. Anyone without the Game page sees only those whose draw delay has passed, as players get them; staff with it see every one at once."`
	Patterns      []GamePattern `json:"patterns" nullable:"false" doc:"The win patterns it is played for, as they were when it started."`
	TotalCalled   int           `json:"totalCalled" doc:"How many numbers calledNumbers holds."`
	YoeverEnabled bool          `json:"yoeverEnabled" doc:"Whether players may send the It's Yoever reaction. On when a game starts."`
	YoeverCount   int           `json:"yoeverCount" doc:"How many times players have sent the reaction this game."`
	AutoEnabled   bool          `json:"autoEnabled" doc:"Whether numbers are being drawn on their own, every autoInterval seconds. A manual draw, a new winner and the half-time prompt switch it off, and it is off when the server starts."`
	AutoInterval  int           `json:"autoInterval" doc:"Seconds between automatic draws, 5 to 600."`
	Halftime      *Halftime     `json:"halftime" doc:"The half-time prompt waiting for staff to answer; null when none is (or for anyone without the Game page)."`
	// MinigameAlerted is whether players have been told about a half-time mini-game.
	MinigameAlerted bool      `json:"minigameAlerted" doc:"Whether players have been told a half-time mini-game is on this game (the board shows it)."`
	LastDraw        *LastDraw `json:"lastDraw" doc:"The last number calledNumbers holds, and when players get it; null before the first draw."`
}

// GamePattern is a win pattern as a game copied it when it started, so an edit or
// a delete since leaves the game as it was.
type GamePattern struct {
	ID          int64  `json:"id" doc:"The pattern's id when the game started; it may have been deleted since."`
	Name        string `json:"name" doc:"The pattern's name when the game started."`
	PatternData Grid   `json:"patternData" doc:"Its 5x5 grid, row by row from the top: true marks a cell the win needs. The centre is the free space."`
}

// Grid is a pattern's 5x5 grid, row by row from the top.
type Grid [][]bool

// Schema is exactly five rows of exactly five booleans, none of them null, as the
// patterns' own grid (internal/api's PatternGrid).
func (Grid) Schema(huma.Registry) *huma.Schema {
	five := 5
	return &huma.Schema{
		Type: huma.TypeArray, MinItems: &five, MaxItems: &five,
		Items: &huma.Schema{
			Type: huma.TypeArray, MinItems: &five, MaxItems: &five,
			Items: &huma.Schema{Type: huma.TypeBoolean},
		},
	}
}

// DrawnNumber is a number a game called.
type DrawnNumber struct {
	Number    int    `json:"number" minimum:"1" maximum:"75" doc:"The number, 1 to 75."`
	Letter    string `json:"letter" enum:"B,I,N,G,O" doc:"Its column's letter: B for 1-15, I 16-30, N 31-45, G 46-60, O 61-75."`
	CallOrder int    `json:"callOrder" minimum:"1" doc:"Its place among the game's calls, from 1."`
}

// LastDraw is the last number a game called, with when players get it.
type LastDraw struct {
	DrawnNumber
	PlayersAt *string `json:"playersAt" nullable:"true" doc:"When players get it, or got it: when it was drawn, plus that draw's delay. RFC 3339 in UTC. Null when the server can't say (it was drawn before the server last started)."`
}

// Halftime is a half-time prompt waiting for an answer.
type Halftime struct {
	GameID     int64 `json:"gameId" doc:"The game whose half-time mark was reached: the answer names it."`
	AutoPaused bool  `json:"autoPaused" doc:"Whether the automatic draws were paused for the prompt, so declining the mini-game starts them again."`
}

// DrawResult is a draw: the number, and the game with it.
type DrawResult struct {
	Drawn   DrawnNumber `json:"drawn" doc:"The number drawn."`
	Game    Game        `json:"game" doc:"The game after the draw."`
	Winners []string    `json:"winners" nullable:"false" doc:"The ids of the cards that have won the game so far, in the order they won."`
}

// EndResult says whether a game was ended.
type EndResult struct {
	Ended  bool  `json:"ended" doc:"True when this ended the game; false when it had already ended (another console, or the plugin, ended it first), so nothing was ended or logged."`
	Logged int64 `json:"logged" doc:"How many winners were written to the winners log."`
}

// YoeverResult is a reaction a player sent.
type YoeverResult struct {
	Count           int `json:"count" doc:"How many times players have sent it this game, this one included."`
	CooldownSeconds int `json:"cooldownSeconds" doc:"Seconds before this card may send it again (the settings' yoeverCooldownSeconds)."`
}
