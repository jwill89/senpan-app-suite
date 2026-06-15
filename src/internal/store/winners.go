package store

import (
	"fmt"

	"app-suite/internal/model"
)

// ── Winners Log operations ──────────────────────────────────────────────────

// InsertWinnersLog inserts multiple winners log entries in a transaction.
func (s *Store) InsertWinnersLog(entries []model.WinnersLogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare("INSERT INTO winners_log (card_id, player_name, game_details, winning_patterns) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range entries {
		if _, err := stmt.Exec(e.CardID, e.PlayerName, e.GameDetails, e.WinningPatterns); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListWinnersLog returns paginated winners log entries with total count.
func (s *Store) ListWinnersLog(limit, offset int, sortField, sortDir string) ([]model.WinnersLogEntry, int, error) {
	// Whitelist sort fields
	allowedFields := map[string]string{
		"logged_at":    "logged_at",
		"card_id":      "card_id",
		"player_name":  "player_name",
		"game_details": "game_details",
	}
	col, ok := allowedFields[sortField]
	if !ok {
		col = "logged_at"
	}
	dir := "DESC"
	if sortDir == "asc" {
		dir = "ASC"
	}

	query := fmt.Sprintf("SELECT id, logged_at, card_id, player_name, game_details, winning_patterns, COUNT(*) OVER() as total FROM winners_log ORDER BY %s %s LIMIT ? OFFSET ?", col, dir)
	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var total int
	entries := make([]model.WinnersLogEntry, 0)
	for rows.Next() {
		var e model.WinnersLogEntry
		if err := rows.Scan(&e.ID, &e.LoggedAt, &e.CardID, &e.PlayerName, &e.GameDetails, &e.WinningPatterns, &total); err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}

// FrequentWinners returns players who have won at least minWins times in the last hours.
func (s *Store) FrequentWinners(minWins, hours int) ([]model.FrequentWinner, error) {
	query := `SELECT player_name, COUNT(*) as win_count FROM winners_log
		WHERE player_name != '' AND logged_at >= datetime('now', '-' || ? || ' hours')
		GROUP BY player_name HAVING COUNT(*) >= ?`
	rows, err := s.db.Query(query, hours, minWins)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	winners := make([]model.FrequentWinner, 0)
	for rows.Next() {
		var w model.FrequentWinner
		if err := rows.Scan(&w.PlayerName, &w.WinCount); err != nil {
			return nil, err
		}
		winners = append(winners, w)
	}
	return winners, rows.Err()
}
