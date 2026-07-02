package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"app-suite/internal/model"
)

// ── Game operations ─────────────────────────────────────────────────────────

// CreateGame inserts a new active game and returns its ID.
func (s *Store) CreateGame() (int64, error) {
	res, err := s.db.Exec("INSERT INTO games (status, winners_cache) VALUES ('active', '[]')")
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// StartGame atomically ends any active game, creates a new active game, and
// snapshots the given patterns into game_patterns — all in one transaction, so a
// failure part-way can't leave a new active game with only a partial pattern set
// (which would silently miscalibrate winner detection). Returns the new game's id
// and created_at timestamp.
func (s *Store) StartGame(patterns []model.BingoGamePattern) (int64, string, error) {
	tx, err := s.beginImmediate()
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("UPDATE games SET status = 'ended' WHERE status = 'active'"); err != nil {
		return 0, "", fmt.Errorf("end active games: %w", err)
	}

	res, err := tx.Exec("INSERT INTO games (status, winners_cache) VALUES ('active', '[]')")
	if err != nil {
		return 0, "", fmt.Errorf("create game: %w", err)
	}
	gameID, err := res.LastInsertId()
	if err != nil {
		return 0, "", err
	}

	stmt, err := tx.Prepare("INSERT INTO game_patterns (game_id, pattern_id, pattern_name, pattern_data) VALUES (?, ?, ?, ?)")
	if err != nil {
		return 0, "", err
	}
	defer stmt.Close()
	for _, p := range patterns {
		jsonData, err := json.Marshal(p.PatternData)
		if err != nil {
			return 0, "", fmt.Errorf("marshal pattern data: %w", err)
		}
		if _, err := stmt.Exec(gameID, p.ID, p.Name, string(jsonData)); err != nil {
			return 0, "", fmt.Errorf("snapshot pattern: %w", err)
		}
	}

	var createdAt string
	if err := tx.QueryRow("SELECT created_at FROM games WHERE id = ?", gameID).Scan(&createdAt); err != nil {
		return 0, "", err
	}

	if err := tx.Commit(); err != nil {
		return 0, "", err
	}
	return gameID, createdAt, nil
}

// EndGame marks a game as ended.
func (s *Store) EndGame(gameID int64) error {
	_, err := s.db.Exec("UPDATE games SET status = 'ended' WHERE id = ?", gameID)
	return err
}

// GetActiveGame returns the most recent active game, or nil if none.
func (s *Store) GetActiveGame() (*model.BingoGame, error) {
	var g model.BingoGame
	err := s.db.QueryRow(
		"SELECT id, status, created_at, winners_cache FROM games WHERE status = 'active' ORDER BY id DESC LIMIT 1",
	).Scan(&g.ID, &g.Status, &g.CreatedAt, &g.WinnersCache)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// AddGamePattern inserts a snapshot of a pattern for a game.
func (s *Store) AddGamePattern(gameID int64, patternID int, name string, data [][]bool) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal pattern data: %w", err)
	}
	_, err = s.db.Exec(
		"INSERT INTO game_patterns (game_id, pattern_id, pattern_name, pattern_data) VALUES (?, ?, ?, ?)",
		gameID, patternID, name, string(jsonData),
	)
	return err
}

// GetGamePatterns retrieves all pattern snapshots for a game.
func (s *Store) GetGamePatterns(gameID int64) ([]model.BingoGamePattern, error) {
	rows, err := s.db.Query(
		"SELECT pattern_id, pattern_name, pattern_data FROM game_patterns WHERE game_id = ?", gameID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	patterns := make([]model.BingoGamePattern, 0)
	for rows.Next() {
		var gp model.BingoGamePattern
		var dataJSON string
		if err := rows.Scan(&gp.ID, &gp.Name, &dataJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(dataJSON), &gp.PatternData); err != nil {
			return nil, fmt.Errorf("unmarshal game pattern: %w", err)
		}
		patterns = append(patterns, gp)
	}
	return patterns, rows.Err()
}

// AddCalledNumber records a drawn number for a game.
func (s *Store) AddCalledNumber(gameID int64, number, order int) error {
	_, err := s.db.Exec(
		"INSERT INTO called_numbers (game_id, number, call_order) VALUES (?, ?, ?)",
		gameID, number, order,
	)
	return err
}

// GetCalledNumbers returns the called numbers for a game in call order.
func (s *Store) GetCalledNumbers(gameID int64) ([]int, error) {
	rows, err := s.db.Query(
		"SELECT number FROM called_numbers WHERE game_id = ? ORDER BY call_order", gameID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	numbers := make([]int, 0)
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		numbers = append(numbers, n)
	}
	return numbers, rows.Err()
}

// UpdateWinnersCache persists computed winner IDs.
func (s *Store) UpdateWinnersCache(gameID int64, winners []string) error {
	if winners == nil {
		winners = []string{}
	}
	data, err := json.Marshal(winners)
	if err != nil {
		return fmt.Errorf("marshal winners: %w", err)
	}
	_, err = s.db.Exec("UPDATE games SET winners_cache = ? WHERE id = ?", string(data), gameID)
	return err
}
