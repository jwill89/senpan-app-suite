package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"app-suite/internal/model"
)

// ── Card operations ─────────────────────────────────────────────────────────

// SaveCard inserts a new card with the given board data.
func (s *Store) SaveCard(id string, board [][]int) error {
	data, err := json.Marshal(board)
	if err != nil {
		return fmt.Errorf("marshal board: %w", err)
	}
	_, err = s.db.Exec(
		"INSERT INTO cards (id, board_data, created_at) VALUES (?, ?, CURRENT_TIMESTAMP)", id, string(data))
	return err
}

// CardBatchEntry holds an ID and board for batch insertion.
type CardBatchEntry struct {
	ID    string
	Board [][]int
}

// SaveCardsBatch inserts multiple cards in a single transaction.
func (s *Store) SaveCardsBatch(cards []CardBatchEntry) error {
	tx, err := s.beginImmediate()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare("INSERT INTO cards (id, board_data, created_at) VALUES (?, ?, CURRENT_TIMESTAMP)")
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	for _, c := range cards {
		data, err := json.Marshal(c.Board)
		if err != nil {
			return fmt.Errorf("marshal board: %w", err)
		}
		if _, err := stmt.Exec(c.ID, string(data)); err != nil {
			return fmt.Errorf("insert card %s: %w", c.ID, err)
		}
	}
	return tx.Commit()
}

// GetCard retrieves a single card by ID. Returns nil if not found.
func (s *Store) GetCard(id string) (*model.Card, error) {
	var card model.Card
	var boardJSON string
	var protected int
	err := s.db.QueryRow("SELECT id, board_data, player_name, details, protected, custom_status, world FROM cards WHERE id = ?", id).
		Scan(&card.ID, &boardJSON, &card.PlayerName, &card.Details, &protected, &card.CustomStatus, &card.World)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	card.Protected = protected != 0
	if err := json.Unmarshal([]byte(boardJSON), &card.BoardData); err != nil {
		return nil, fmt.Errorf("unmarshal board: %w", err)
	}
	return &card, nil
}

// ListCards retrieves all cards with decoded board data.
func (s *Store) ListCards() ([]model.Card, error) {
	rows, err := s.db.Query("SELECT id, board_data, player_name, details, protected, custom_status, world FROM cards ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cards := make([]model.Card, 0)
	for rows.Next() {
		var c model.Card
		var boardJSON string
		var protected int
		if err := rows.Scan(&c.ID, &boardJSON, &c.PlayerName, &c.Details, &protected, &c.CustomStatus, &c.World); err != nil {
			return nil, err
		}
		c.Protected = protected != 0
		if err := json.Unmarshal([]byte(boardJSON), &c.BoardData); err != nil {
			return nil, fmt.Errorf("unmarshal board: %w", err)
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// ListCardIDs returns only card IDs (lightweight endpoint).
func (s *Store) ListCardIDs() ([]string, error) {
	rows, err := s.db.Query("SELECT id FROM cards ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CardExists checks whether a card with the given ID exists.
func (s *Store) CardExists(id string) (bool, error) {
	var n int
	err := s.db.QueryRow("SELECT 1 FROM cards WHERE id = ?", id).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// DeleteCard removes a card by ID. Returns true if a row was deleted.
func (s *Store) DeleteCard(id string) (bool, error) {
	res, err := s.db.Exec("DELETE FROM cards WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteAllCards removes every non-Protected card and returns the IDs it deleted.
// Protected cards (approved custom cards, or any card an admin has marked Protected)
// are spared — they can only be removed individually via DeleteCard.
func (s *Store) DeleteAllCards() ([]string, error) {
	rows, err := s.db.Query("DELETE FROM cards WHERE protected = 0 RETURNING id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SetCardProtected marks or unmarks a card as Protected. Returns true if the card exists.
func (s *Store) SetCardProtected(id string, protected bool) (bool, error) {
	res, err := s.db.Exec("UPDATE cards SET protected = ? WHERE id = ?", boolToInt(protected), id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ApproveCustomCard approves a pending custom card: it becomes live ('approved') and
// is automatically Protected. Returns true only if a still-pending card with that id
// existed (approving a normal or already-approved card is a no-op → false).
func (s *Store) ApproveCustomCard(id string) (bool, error) {
	res, err := s.db.Exec(
		"UPDATE cards SET custom_status = 'approved', protected = 1 WHERE id = ? AND custom_status = 'pending'", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CreateCustomCard inserts a Personal Card Request as a pending custom card — stored
// but not yet playable until an admin approves it. The character name goes in
// player_name and the home world in world; board is the 5×5 grid the requester built.
func (s *Store) CreateCustomCard(id string, board [][]int, characterName, world string) error {
	data, err := json.Marshal(board)
	if err != nil {
		return fmt.Errorf("marshal board: %w", err)
	}
	_, err = s.db.Exec(
		"INSERT INTO cards (id, board_data, player_name, world, custom_status, protected, created_at) VALUES (?, ?, ?, ?, 'pending', 0, CURRENT_TIMESTAMP)",
		id, string(data), characterName, world,
	)
	return err
}

// FindDuplicateBoard returns the id of an existing card whose numbers exactly match
// board (same numbers in the same cells), or ("", false) when none does. Every write
// path stores board_data as the compact, deterministic row-major JSON that
// json.Marshal produces for a [][]int, so the stored value is already canonical and
// can be matched directly in SQL — no need to pull every row into Go and re-marshal
// each one on the public custom-card duplicate check.
func (s *Store) FindDuplicateBoard(board [][]int) (string, bool, error) {
	target, err := json.Marshal(board)
	if err != nil {
		return "", false, fmt.Errorf("marshal board: %w", err)
	}
	var id string
	err = s.db.QueryRow("SELECT id FROM cards WHERE board_data = ? LIMIT 1", string(target)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

// UpdateCardPlayer sets the player_name and details for a card.
func (s *Store) UpdateCardPlayer(id, playerName, details string) error {
	_, err := s.db.Exec("UPDATE cards SET player_name = ?, details = ? WHERE id = ?", playerName, details, id)
	return err
}

// ListCardIDsWithNames returns card IDs along with player names, details, the
// creation timestamp (blank for cards created before the column was added), and the
// status columns (protected / custom_status / world) the admin Manage Cards table
// renders as icons.
func (s *Store) ListCardIDsWithNames() ([]model.Card, error) {
	rows, err := s.db.Query(
		"SELECT id, player_name, details, COALESCE(created_at, ''), protected, custom_status, world FROM cards ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cards := make([]model.Card, 0)
	for rows.Next() {
		var c model.Card
		var protected int
		if err := rows.Scan(&c.ID, &c.PlayerName, &c.Details, &c.CreatedAt, &protected, &c.CustomStatus, &c.World); err != nil {
			return nil, err
		}
		c.Protected = protected != 0
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// GetCardPlayerNames returns a map of card ID → player name for the given IDs.
func (s *Store) GetCardPlayerNames(ids []string) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf("SELECT id, player_name FROM cards WHERE id IN (%s)", strings.Join(placeholders, ","))
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string, len(ids))
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		result[id] = name
	}
	return result, rows.Err()
}
