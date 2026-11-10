package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// The patterns' API (internal/api, patterns.go) and the game (internal/game) read
// and write them through sqlc; what is left here is the tests' way of making one.

// SavePattern inserts a new pattern last in its category and returns its ID. A
// categoryID of 0 or less is the seeded first category (1). For tests that need a
// pattern to start a game with; the API makes them otherwise.
func (s *Store) SavePattern(ctx context.Context, name string, data [][]bool, categoryID int64) (int64, error) {
	if categoryID <= 0 {
		categoryID = 1 // default to "Standard"
	}
	var nextOrder int
	if err := s.read.QueryRowContext(ctx, "SELECT COALESCE(MAX(sort_order), 0) + 1 FROM patterns WHERE category_id = ?", categoryID).Scan(&nextOrder); err != nil {
		return 0, err
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return 0, fmt.Errorf("marshal pattern: %w", err)
	}
	res, err := s.write.ExecContext(ctx,
		"INSERT INTO patterns (name, pattern_data, sort_order, category_id) VALUES (?, ?, ?, ?)",
		name, string(jsonData), nextOrder, categoryID,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
