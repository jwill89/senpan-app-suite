package model

// BoardResponse is the full body of GET /api/board: the card plus the current
// game state and game details. (game/game_details are always written.)
type BoardResponse struct {
	Card        Card            `json:"card"`
	Game        *BingoGameState `json:"game"`
	GameDetails string          `json:"game_details"`
}

// CardResponse is the preview body of GET /api/board?preview=1: only the card.
type CardResponse struct {
	Card Card `json:"card"`
}
