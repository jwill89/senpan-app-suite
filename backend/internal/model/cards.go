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

// CardListEntry is the lightweight card shape from GET /api/cards (no board_data).
type CardListEntry struct {
	ID         string `json:"id"`
	PlayerName string `json:"player_name"`
	Details    string `json:"details"`
	CreatedAt  string `json:"created_at"`
}

// CardsListResponse is the body of GET /api/cards.
type CardsListResponse struct {
	Cards []CardListEntry `json:"cards"`
}

// GeneratedCard is one freshly generated card (id + board) from a bulk generate.
type GeneratedCard struct {
	ID        string  `json:"id"`
	BoardData [][]int `json:"board_data"`
}

// GenerateCardsResponse is the body of POST /api/cards/generate.
type GenerateCardsResponse struct {
	Cards []GeneratedCard `json:"cards"`
	Count int             `json:"count"`
}

// GeneratedNamedCard is the single card created by POST /api/cards (carrying the
// optionally-assigned player name).
type GeneratedNamedCard struct {
	ID         string  `json:"id"`
	PlayerName string  `json:"player_name"`
	BoardData  [][]int `json:"board_data"`
}

// GenerateSingleCardResponse is the body of POST /api/cards.
type GenerateSingleCardResponse struct {
	Card  GeneratedNamedCard `json:"card"`
	Count int                `json:"count"`
}
