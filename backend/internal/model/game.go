package model

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

// BingoGamePattern is a point-in-time snapshot of a pattern attached to a bingo game.
// Snapshots are stored so that deleting or renaming a pattern does not
// affect games that were started with it.
type BingoGamePattern struct {
	ID          int      `json:"id"`           // original pattern ID at time of snapshot
	Name        string   `json:"name"`         // pattern name at time of snapshot
	PatternData [][]bool `json:"pattern_data"` // 5×5 boolean grid
}

// BingoDrawnNumber holds details of a single drawn bingo number, returned after each draw action.
type BingoDrawnNumber struct {
	Number    int    `json:"number"`     // the bingo number (1–75)
	Letter    string `json:"letter"`     // column letter: B, I, N, G, or O
	CallOrder int    `json:"call_order"` // 1-based position in the draw sequence
}

// GameStateResponse is the body of GET /api/game and POST /api/game/start.
type GameStateResponse struct {
	Game        *BingoGameState `json:"game"`
	Winners     []string        `json:"winners"`
	GameDetails string          `json:"game_details"`
}

// DrawResult is the body of POST /api/game/draw: the number just drawn,
// the updated game state, and the current winning card IDs.
type DrawResult struct {
	Drawn   BingoDrawnNumber `json:"drawn"`
	Game    BingoGameState   `json:"game"`
	Winners []string         `json:"winners"`
}

// EndGameResponse is the body of POST /api/game/end.
type EndGameResponse struct {
	Ended bool `json:"ended"`
}
