package model

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

// WinnersLogResponse is the body of GET /api/winners-log: a page of entries
// plus the pagination metadata.
type WinnersLogResponse struct {
	Entries []WinnersLogEntry `json:"entries"`
	Total   int               `json:"total"`
	Page    int               `json:"page"`
	PerPage int               `json:"per_page"`
}

// FrequentWinnersResponse is the body of GET /api/winners-log/frequent.
type FrequentWinnersResponse struct {
	Winners []FrequentWinner `json:"winners"`
}
