package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// The cards' API (internal/api, cards.go) and the game (internal/game) read and write
// them through sqlc; what is left here is the tests' way of making a card and of
// checking that one exists.

// SaveCard inserts a new card with the given board. For tests that need a card with
// known numbers; the API makes them otherwise.
func (s *Store) SaveCard(ctx context.Context, id string, board [][]int) error {
	data, err := json.Marshal(board)
	if err != nil {
		return fmt.Errorf("marshal board: %w", err)
	}
	_, err = s.write.ExecContext(ctx,
		"INSERT INTO cards (id, board_data, created_at) VALUES (?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))", id, string(data))
	return err
}

// CardExists checks whether a card with the given ID exists.
func (s *Store) CardExists(ctx context.Context, id string) (bool, error) {
	var n int
	err := s.read.QueryRowContext(ctx, "SELECT 1 FROM cards WHERE id = ?", id).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
