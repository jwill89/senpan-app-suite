package model

// Card represents a bingo card stored in the database.
// BoardData is a 5×5 grid where board[row][col] holds the number;
// col 0=B(1–15), col 1=I(16–30), col 2=N(31–45), col 3=G(46–60), col 4=O(61–75).
// The centre cell [2][2] is always 0, representing the FREE space.
type Card struct {
	ID         string  `json:"id"`          // 6-char alphanumeric unique identifier
	BoardData  [][]int `json:"board_data"`  // 5×5 grid of numbers (0 = FREE)
	PlayerName string  `json:"player_name"` // optional player name assigned by admin / custom-card character name
	Details    string  `json:"details"`     // optional extra info about the cardholder
	CreatedAt  string  `json:"created_at"`  // ISO timestamp the card was generated ("" if pre-dates tracking)
	// Protected cards are spared by the admin "Delete All" action (they can still be
	// deleted individually). Approved custom cards are automatically Protected.
	Protected bool `json:"protected"`
	// CustomStatus is the custom-card lifecycle: "" (a normal generated card),
	// "pending" (a Personal Card Request awaiting staff approval — not yet playable),
	// or "approved" (an approved, live custom card).
	CustomStatus string `json:"custom_status"`
	// World is the requester's home world for a custom-card request ("" otherwise).
	World string `json:"world"`
}

// CardListEntry is the lightweight card shape from GET /api/cards (no board_data).
type CardListEntry struct {
	ID           string `json:"id"`
	PlayerName   string `json:"player_name"`
	Details      string `json:"details"`
	CreatedAt    string `json:"created_at"`
	Protected    bool   `json:"protected"`
	CustomStatus string `json:"custom_status"`
	World        string `json:"world"`
}

// CardRequestResponse is the body of a successful POST /api/cards/request: the
// custom card was accepted and stored as pending, awaiting staff approval.
type CardRequestResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"` // always "pending" on success
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
