// Package store provides all database access through a single Store type.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"app-suite/internal/model"

	_ "github.com/ncruces/go-sqlite3/driver"
)

// randInt returns a random int in [0, n).
func randInt(n int) int { return rand.IntN(n) }

// Store wraps the SQLite database connection and provides typed CRUD methods.
type Store struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database at path, applies migrations,
// and returns a ready-to-use Store.
func New(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// SQLite only supports a single writer; limit open connections to avoid
	// lock contention with the pure-Go driver.
	db.SetMaxOpenConns(4)

	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA cache_size = -8000",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA mmap_size = 33554432",
		"PRAGMA foreign_keys = ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("set pragma %q: %w", p, err)
		}
	}

	if err := ensureSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensure schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }

// ── Card operations ─────────────────────────────────────────────────────────

// SaveCard inserts a new card with the given board data.
func (s *Store) SaveCard(id string, board [][]int) error {
	data, err := json.Marshal(board)
	if err != nil {
		return fmt.Errorf("marshal board: %w", err)
	}
	_, err = s.db.Exec("INSERT INTO cards (id, board_data) VALUES (?, ?)", id, string(data))
	return err
}

// CardBatchEntry holds an ID and board for batch insertion.
type CardBatchEntry struct {
	ID    string
	Board [][]int
}

// SaveCardsBatch inserts multiple cards in a single transaction.
func (s *Store) SaveCardsBatch(cards []CardBatchEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare("INSERT INTO cards (id, board_data) VALUES (?, ?)")
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
	err := s.db.QueryRow("SELECT id, board_data, player_name, details FROM cards WHERE id = ?", id).
		Scan(&card.ID, &boardJSON, &card.PlayerName, &card.Details)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(boardJSON), &card.BoardData); err != nil {
		return nil, fmt.Errorf("unmarshal board: %w", err)
	}
	return &card, nil
}

// ListCards retrieves all cards with decoded board data.
func (s *Store) ListCards() ([]model.Card, error) {
	rows, err := s.db.Query("SELECT id, board_data, player_name, details FROM cards ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cards := make([]model.Card, 0)
	for rows.Next() {
		var c model.Card
		var boardJSON string
		if err := rows.Scan(&c.ID, &boardJSON, &c.PlayerName, &c.Details); err != nil {
			return nil, err
		}
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

// DeleteAllCards removes all cards. Returns the count deleted.
func (s *Store) DeleteAllCards() (int64, error) {
	res, err := s.db.Exec("DELETE FROM cards")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// UpdateCardPlayer sets the player_name and details for a card.
func (s *Store) UpdateCardPlayer(id, playerName, details string) error {
	_, err := s.db.Exec("UPDATE cards SET player_name = ?, details = ? WHERE id = ?", playerName, details, id)
	return err
}

// ListCardIDsWithNames returns card IDs along with player names.
func (s *Store) ListCardIDsWithNames() ([]model.Card, error) {
	rows, err := s.db.Query("SELECT id, player_name, details FROM cards ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cards := make([]model.Card, 0)
	for rows.Next() {
		var c model.Card
		if err := rows.Scan(&c.ID, &c.PlayerName, &c.Details); err != nil {
			return nil, err
		}
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

// ── Pattern Category operations ─────────────────────────────────────────────

// ListPatternCategories returns all pattern categories ordered by sort_order.
func (s *Store) ListPatternCategories() ([]model.PatternCategory, error) {
	rows, err := s.db.Query("SELECT id, name, sort_order FROM pattern_categories ORDER BY sort_order ASC, id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cats := make([]model.PatternCategory, 0)
	for rows.Next() {
		var c model.PatternCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.SortOrder); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

// CreatePatternCategory inserts a new category and returns its ID.
func (s *Store) CreatePatternCategory(name string) (int64, error) {
	var nextOrder int
	if err := s.db.QueryRow("SELECT COALESCE(MAX(sort_order), 0) + 1 FROM pattern_categories").Scan(&nextOrder); err != nil {
		return 0, err
	}
	res, err := s.db.Exec("INSERT INTO pattern_categories (name, sort_order) VALUES (?, ?)", name, nextOrder)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RenamePatternCategory updates a category's name. Returns true if updated.
func (s *Store) RenamePatternCategory(id int64, name string) (bool, error) {
	res, err := s.db.Exec("UPDATE pattern_categories SET name = ? WHERE id = ?", name, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeletePatternCategory deletes a category and reassigns its patterns to the
// default category (lowest sort_order). Returns false if the category doesn't
// exist or is the last remaining category.
func (s *Store) DeletePatternCategory(id int64) (bool, error) {
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM pattern_categories").Scan(&count); err != nil {
		return false, err
	}
	if count <= 1 {
		return false, nil // don't delete the last category
	}

	// Find the fallback category (lowest sort_order that isn't the one being deleted).
	var fallbackID int64
	err := s.db.QueryRow(
		"SELECT id FROM pattern_categories WHERE id != ? ORDER BY sort_order ASC, id ASC LIMIT 1", id,
	).Scan(&fallbackID)
	if err != nil {
		return false, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	// Reassign patterns to fallback.
	if _, err := tx.Exec("UPDATE patterns SET category_id = ? WHERE category_id = ?", fallbackID, id); err != nil {
		return false, err
	}
	res, err := tx.Exec("DELETE FROM pattern_categories WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, nil
	}
	return true, tx.Commit()
}

// MovePatternCategory swaps a category up or down.
func (s *Store) MovePatternCategory(id int64, direction string) (bool, error) {
	rows, err := s.db.Query("SELECT id FROM pattern_categories ORDER BY sort_order ASC, id ASC")
	if err != nil {
		return false, err
	}
	defer rows.Close()

	var ordered []int64
	for rows.Next() {
		var cid int64
		if err := rows.Scan(&cid); err != nil {
			return false, err
		}
		ordered = append(ordered, cid)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}

	idx := -1
	for i, cid := range ordered {
		if cid == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return false, nil
	}

	swapIdx := idx - 1
	if direction == "down" {
		swapIdx = idx + 1
	}
	if swapIdx < 0 || swapIdx >= len(ordered) {
		return false, nil
	}

	return s.swapCategoryOrder(ordered[idx], ordered[swapIdx])
}

func (s *Store) swapCategoryOrder(idA, idB int64) (bool, error) {
	rows, err := s.db.Query("SELECT id, sort_order FROM pattern_categories WHERE id IN (?, ?)", idA, idB)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	orderMap := make(map[int64]int, 2)
	for rows.Next() {
		var id int64
		var sortOrder int
		if err := rows.Scan(&id, &sortOrder); err != nil {
			return false, err
		}
		orderMap[id] = sortOrder
	}
	if len(orderMap) != 2 {
		return false, nil
	}

	orderA, orderB := orderMap[idA], orderMap[idB]
	if orderA == orderB {
		orderB = orderA + 1
	}

	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("UPDATE pattern_categories SET sort_order = ? WHERE id = ?", orderB, idA); err != nil {
		return false, err
	}
	if _, err := tx.Exec("UPDATE pattern_categories SET sort_order = ? WHERE id = ?", orderA, idB); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// ── Pattern operations ──────────────────────────────────────────────────────

// SavePattern inserts a new pattern and returns its ID.
func (s *Store) SavePattern(name string, data [][]bool, categoryID int64) (int64, error) {
	if categoryID <= 0 {
		categoryID = 1 // default to "Standard"
	}
	var nextOrder int
	if err := s.db.QueryRow("SELECT COALESCE(MAX(sort_order), 0) + 1 FROM patterns WHERE category_id = ?", categoryID).Scan(&nextOrder); err != nil {
		return 0, err
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return 0, fmt.Errorf("marshal pattern: %w", err)
	}
	res, err := s.db.Exec(
		"INSERT INTO patterns (name, pattern_data, sort_order, category_id) VALUES (?, ?, ?, ?)",
		name, string(jsonData), nextOrder, categoryID,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DuplicatePatternInfo holds info about an existing pattern that matches.
type DuplicatePatternInfo struct {
	ID           int64
	Name         string
	CategoryName string
}

// FindDuplicatePattern checks if a pattern with identical pattern_data already exists.
// Returns nil if no duplicate is found.
func (s *Store) FindDuplicatePattern(data [][]bool) (*DuplicatePatternInfo, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal pattern: %w", err)
	}
	var info DuplicatePatternInfo
	err = s.db.QueryRow(`
		SELECT p.id, p.name, pc.name
		FROM patterns p
		JOIN pattern_categories pc ON pc.id = p.category_id
		WHERE p.pattern_data = ?`, string(jsonData)).Scan(&info.ID, &info.Name, &info.CategoryName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// ListPatterns returns all patterns ordered by category sort_order, then pattern sort_order.
func (s *Store) ListPatterns() ([]model.Pattern, error) {
	rows, err := s.db.Query(`
		SELECT p.id, p.name, p.pattern_data, p.sort_order, p.category_id, pc.name
		FROM patterns p
		JOIN pattern_categories pc ON pc.id = p.category_id
		ORDER BY pc.sort_order ASC, pc.id ASC, p.sort_order ASC, p.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	patterns := make([]model.Pattern, 0)
	for rows.Next() {
		var p model.Pattern
		var dataJSON string
		if err := rows.Scan(&p.ID, &p.Name, &dataJSON, &p.SortOrder, &p.CategoryID, &p.CategoryName); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(dataJSON), &p.PatternData); err != nil {
			return nil, fmt.Errorf("unmarshal pattern: %w", err)
		}
		patterns = append(patterns, p)
	}
	return patterns, rows.Err()
}

// GetPatternsByIDs returns patterns matching the given IDs.
func (s *Store) GetPatternsByIDs(ids []int) ([]model.Pattern, error) {
	if len(ids) == 0 {
		return []model.Pattern{}, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(
		"SELECT id, name, pattern_data FROM patterns WHERE id IN (%s)",
		strings.Join(placeholders, ","),
	)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	patterns := make([]model.Pattern, 0)
	for rows.Next() {
		var p model.Pattern
		var dataJSON string
		if err := rows.Scan(&p.ID, &p.Name, &dataJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(dataJSON), &p.PatternData); err != nil {
			return nil, fmt.Errorf("unmarshal pattern: %w", err)
		}
		patterns = append(patterns, p)
	}
	return patterns, rows.Err()
}

// RenamePattern updates a pattern's name. Returns true if a row was updated.
func (s *Store) RenamePattern(id int, name string) (bool, error) {
	res, err := s.db.Exec("UPDATE patterns SET name = ? WHERE id = ?", name, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeletePattern removes a pattern by ID. Returns true if a row was deleted.
func (s *Store) DeletePattern(id int) (bool, error) {
	res, err := s.db.Exec("DELETE FROM patterns WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// MovePattern moves a pattern up or down within its category.
// Returns true if the swap was performed.
func (s *Store) MovePattern(id int, direction string) (bool, error) {
	// Find the pattern's category.
	var categoryID int64
	err := s.db.QueryRow("SELECT category_id FROM patterns WHERE id = ?", id).Scan(&categoryID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	// Get ordered IDs within the same category.
	rows, err := s.db.Query("SELECT id FROM patterns WHERE category_id = ? ORDER BY sort_order ASC, id ASC", categoryID)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	var ordered []int
	for rows.Next() {
		var pid int
		if err := rows.Scan(&pid); err != nil {
			return false, err
		}
		ordered = append(ordered, pid)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}

	idx := slices.Index(ordered, id)
	if idx == -1 {
		return false, nil
	}

	swapIdx := idx - 1
	if direction == "down" {
		swapIdx = idx + 1
	}
	if swapIdx < 0 || swapIdx >= len(ordered) {
		return false, nil // already at boundary
	}

	return s.swapPatternOrder(ordered[idx], ordered[swapIdx])
}

// swapPatternOrder exchanges the sort_order of two patterns in a transaction.
func (s *Store) swapPatternOrder(idA, idB int) (bool, error) {
	rows, err := s.db.Query("SELECT id, sort_order FROM patterns WHERE id IN (?, ?)", idA, idB)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	orderMap := make(map[int]int, 2)
	for rows.Next() {
		var id, sortOrder int
		if err := rows.Scan(&id, &sortOrder); err != nil {
			return false, err
		}
		orderMap[id] = sortOrder
	}
	if len(orderMap) != 2 {
		return false, nil
	}

	orderA, orderB := orderMap[idA], orderMap[idB]
	if orderA == orderB {
		orderB = orderA + 1 // force apart when equal
	}

	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("UPDATE patterns SET sort_order = ? WHERE id = ?", orderB, idA); err != nil {
		return false, err
	}
	if _, err := tx.Exec("UPDATE patterns SET sort_order = ? WHERE id = ?", orderA, idB); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// SetPatternCategory moves a pattern to a different category.
// The pattern is placed at the end of the target category's sort order.
func (s *Store) SetPatternCategory(patternID int, categoryID int64) error {
	var nextOrder int
	if err := s.db.QueryRow("SELECT COALESCE(MAX(sort_order), 0) + 1 FROM patterns WHERE category_id = ?", categoryID).Scan(&nextOrder); err != nil {
		return err
	}
	_, err := s.db.Exec("UPDATE patterns SET category_id = ?, sort_order = ? WHERE id = ?", categoryID, nextOrder, patternID)
	return err
}

// BulkReorderCategories sets the sort_order for each category based on the provided ordered list of IDs.
func (s *Store) BulkReorderCategories(ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, id := range ids {
		if _, err := tx.Exec("UPDATE pattern_categories SET sort_order = ? WHERE id = ?", i, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// BulkReorderPatterns sets the sort_order for each pattern within a given category based on the provided ordered list of IDs.
func (s *Store) BulkReorderPatterns(categoryID int64, ids []int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, id := range ids {
		if _, err := tx.Exec("UPDATE patterns SET sort_order = ?, category_id = ? WHERE id = ?", i, categoryID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ── Game operations ─────────────────────────────────────────────────────────

// CreateGame inserts a new active game and returns its ID.
func (s *Store) CreateGame() (int64, error) {
	res, err := s.db.Exec("INSERT INTO games (status, winners_cache) VALUES ('active', '[]')")
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
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

// ── Settings operations ─────────────────────────────────────────────────────

// GetSetting retrieves a setting value by key. Returns empty string if not found.
func (s *Store) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

// SetSetting upserts a setting value by key.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		"INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	return err
}

// ── Style operations ────────────────────────────────────────────────────────

// ListStyles returns all styles (without CSS content for lightweight listing).
func (s *Store) ListStyles() ([]model.Style, error) {
	rows, err := s.db.Query("SELECT id, name, created_at FROM styles ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	styles := make([]model.Style, 0)
	for rows.Next() {
		var st model.Style
		if err := rows.Scan(&st.ID, &st.Name, &st.CreatedAt); err != nil {
			return nil, err
		}
		styles = append(styles, st)
	}
	return styles, rows.Err()
}

// GetStyle retrieves a single style by ID. Returns nil if not found.
func (s *Store) GetStyle(id int64) (*model.Style, error) {
	var st model.Style
	err := s.db.QueryRow(
		"SELECT id, name, css_content, created_at FROM styles WHERE id = ?", id,
	).Scan(&st.ID, &st.Name, &st.CSSContent, &st.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// CreateStyle inserts a new style and returns its ID.
func (s *Store) CreateStyle(name, css string) (int64, error) {
	res, err := s.db.Exec(
		"INSERT INTO styles (name, css_content) VALUES (?, ?)", name, css,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateStyle updates a style's name and CSS content.
func (s *Store) UpdateStyle(id int64, name, css string) error {
	_, err := s.db.Exec(
		"UPDATE styles SET name = ?, css_content = ? WHERE id = ?", name, css, id,
	)
	return err
}

// DeleteStyle removes a style by ID. Returns true if a row was deleted.
func (s *Store) DeleteStyle(id int64) (bool, error) {
	res, err := s.db.Exec("DELETE FROM styles WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GetActiveStyleCSS returns the CSS content of the currently active style.
// Returns empty string if no active style is set or the style doesn't exist.
func (s *Store) GetActiveStyleCSS() (string, error) {
	idStr, err := s.GetSetting("active_style_id")
	if err != nil || idStr == "" {
		return "", err
	}
	var css string
	err = s.db.QueryRow("SELECT css_content FROM styles WHERE id = ?", idStr).Scan(&css)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return css, err
}

// ── Game preset operations ──────────────────────────────────────────────────

// ListGamePresets returns all game presets ordered by name.
func (s *Store) ListGamePresets() ([]model.GamePreset, error) {
	rows, err := s.db.Query("SELECT id, name, pattern_ids, game_details, created_at FROM game_presets ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	presets := make([]model.GamePreset, 0)
	for rows.Next() {
		var p model.GamePreset
		var idsJSON string
		if err := rows.Scan(&p.ID, &p.Name, &idsJSON, &p.GameDetails, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.PatternIDs = parsePresetPatternIDs(idsJSON)
		presets = append(presets, p)
	}
	return presets, rows.Err()
}

// GetGamePreset retrieves a single game preset by ID. Returns nil if not found.
func (s *Store) GetGamePreset(id int64) (*model.GamePreset, error) {
	var p model.GamePreset
	var idsJSON string
	err := s.db.QueryRow(
		"SELECT id, name, pattern_ids, game_details, created_at FROM game_presets WHERE id = ?", id,
	).Scan(&p.ID, &p.Name, &idsJSON, &p.GameDetails, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.PatternIDs = parsePresetPatternIDs(idsJSON)
	return &p, nil
}

// CreateGamePreset inserts a new game preset and returns its ID.
func (s *Store) CreateGamePreset(name string, patternIDs []int64, gameDetails string) (int64, error) {
	idsJSON, err := json.Marshal(normalizePresetIDs(patternIDs))
	if err != nil {
		return 0, fmt.Errorf("marshal preset pattern ids: %w", err)
	}
	res, err := s.db.Exec(
		"INSERT INTO game_presets (name, pattern_ids, game_details) VALUES (?, ?, ?)",
		name, string(idsJSON), gameDetails,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateGamePreset updates a preset's name, pattern IDs, and game details.
func (s *Store) UpdateGamePreset(id int64, name string, patternIDs []int64, gameDetails string) error {
	idsJSON, err := json.Marshal(normalizePresetIDs(patternIDs))
	if err != nil {
		return fmt.Errorf("marshal preset pattern ids: %w", err)
	}
	_, err = s.db.Exec(
		"UPDATE game_presets SET name = ?, pattern_ids = ?, game_details = ? WHERE id = ?",
		name, string(idsJSON), gameDetails, id,
	)
	return err
}

// DeleteGamePreset removes a preset by ID. Returns true if a row was deleted.
func (s *Store) DeleteGamePreset(id int64) (bool, error) {
	res, err := s.db.Exec("DELETE FROM game_presets WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// normalizePresetIDs guarantees a non-nil slice so it marshals to "[]" not "null".
func normalizePresetIDs(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}

// parsePresetPatternIDs deserializes the JSON pattern_ids column into an int64
// slice. Returns an empty slice on empty input or parse failure.
func parsePresetPatternIDs(raw string) []int64 {
	if raw == "" {
		return []int64{}
	}
	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil || ids == nil {
		return []int64{}
	}
	return ids
}

// ── Raffle operations ───────────────────────────────────────────────────────

// CreateRaffle inserts a new raffle and returns its ID.
func (s *Store) CreateRaffle(r *model.Raffle) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO raffles (title, description, rules, max_entries, signup_instructions, cost_per_entry, available_from, available_to, prize_image, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Title, r.Description, r.Rules, r.MaxEntries, r.SignupInstructions,
		r.CostPerEntry, r.AvailableFrom, r.AvailableTo, r.PrizeImage, "open",
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateRaffle updates an existing raffle's editable fields.
func (s *Store) UpdateRaffle(r *model.Raffle) error {
	_, err := s.db.Exec(`UPDATE raffles SET title=?, description=?, rules=?, max_entries=?, signup_instructions=?, cost_per_entry=?, available_from=?, available_to=?, prize_image=? WHERE id=?`,
		r.Title, r.Description, r.Rules, r.MaxEntries, r.SignupInstructions,
		r.CostPerEntry, r.AvailableFrom, r.AvailableTo, r.PrizeImage, r.ID,
	)
	return err
}

// DeleteRaffle removes a raffle and its entries (cascade). Returns true if deleted.
func (s *Store) DeleteRaffle(id int64) (bool, error) {
	res, err := s.db.Exec("DELETE FROM raffles WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GetRaffle retrieves a single raffle by ID. Returns nil if not found.
func (s *Store) GetRaffle(id int64) (*model.Raffle, error) {
	var r model.Raffle
	var winnerID sql.NullInt64
	err := s.db.QueryRow(`SELECT id, title, description, rules, max_entries, signup_instructions, cost_per_entry, available_from, available_to, prize_image, status, winner_entry_id, created_at FROM raffles WHERE id = ?`, id).
		Scan(&r.ID, &r.Title, &r.Description, &r.Rules, &r.MaxEntries, &r.SignupInstructions,
			&r.CostPerEntry, &r.AvailableFrom, &r.AvailableTo, &r.PrizeImage, &r.Status, &winnerID, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if winnerID.Valid {
		r.WinnerEntryID = &winnerID.Int64
	}
	return &r, nil
}

// ListRaffles returns raffles. If adminMode is false, returns open raffles within the
// availability date range plus all closed raffles (for browsing past results).
func (s *Store) ListRaffles(adminMode bool) ([]model.Raffle, error) {
	var query string
	var args []any
	if adminMode {
		query = `SELECT id, title, description, rules, max_entries, signup_instructions, cost_per_entry, available_from, available_to, prize_image, status, winner_entry_id, created_at FROM raffles ORDER BY created_at DESC`
	} else {
		query = `SELECT id, title, description, rules, max_entries, signup_instructions, cost_per_entry, available_from, available_to, prize_image, status, winner_entry_id, created_at FROM raffles WHERE status = 'open' AND (available_from = '' OR REPLACE(available_from, 'T', ' ') <= datetime('now')) AND (available_to = '' OR REPLACE(available_to, 'T', ' ') >= datetime('now')) ORDER BY created_at DESC`
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	raffles := make([]model.Raffle, 0)
	for rows.Next() {
		var r model.Raffle
		var winnerID sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Title, &r.Description, &r.Rules, &r.MaxEntries, &r.SignupInstructions,
			&r.CostPerEntry, &r.AvailableFrom, &r.AvailableTo, &r.PrizeImage, &r.Status, &winnerID, &r.CreatedAt); err != nil {
			return nil, err
		}
		if winnerID.Valid {
			r.WinnerEntryID = &winnerID.Int64
		}
		raffles = append(raffles, r)
	}
	return raffles, rows.Err()
}

// CountRaffleEntries returns the total number of entries (sum of num_entries) for a raffle.
func (s *Store) CountRaffleEntries(raffleID int64) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COALESCE(SUM(num_entries), 0) FROM raffle_entries WHERE raffle_id = ?`, raffleID).Scan(&count)
	return count, err
}

// ListRaffleEntries returns all entries for a raffle.
func (s *Store) ListRaffleEntries(raffleID int64) ([]model.RaffleEntry, error) {
	rows, err := s.db.Query(`SELECT id, raffle_id, character_name, world, num_entries, paid, created_at FROM raffle_entries WHERE raffle_id = ? ORDER BY created_at ASC`, raffleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]model.RaffleEntry, 0)
	for rows.Next() {
		var e model.RaffleEntry
		var paid int
		if err := rows.Scan(&e.ID, &e.RaffleID, &e.CharacterName, &e.World, &e.NumEntries, &paid, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Paid = paid != 0
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetRaffleEntry retrieves a single raffle entry by character+world for a raffle.
func (s *Store) GetRaffleEntry(raffleID int64, charName, world string) (*model.RaffleEntry, error) {
	var e model.RaffleEntry
	var paid int
	err := s.db.QueryRow(`SELECT id, raffle_id, character_name, world, num_entries, paid, created_at FROM raffle_entries WHERE raffle_id = ? AND LOWER(character_name) = LOWER(?) AND LOWER(world) = LOWER(?)`,
		raffleID, charName, world).
		Scan(&e.ID, &e.RaffleID, &e.CharacterName, &e.World, &e.NumEntries, &paid, &e.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.Paid = paid != 0
	return &e, nil
}

// GetRaffleEntryByID returns a single raffle entry by its ID.
func (s *Store) GetRaffleEntryByID(entryID int64) (*model.RaffleEntry, error) {
	var e model.RaffleEntry
	var paid int
	err := s.db.QueryRow(`SELECT id, raffle_id, character_name, world, num_entries, paid, created_at FROM raffle_entries WHERE id = ?`,
		entryID).
		Scan(&e.ID, &e.RaffleID, &e.CharacterName, &e.World, &e.NumEntries, &paid, &e.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.Paid = paid != 0
	return &e, nil
}

// CreateRaffleEntry inserts a new raffle entry.
func (s *Store) CreateRaffleEntry(raffleID int64, charName, world string, numEntries int) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO raffle_entries (raffle_id, character_name, world, num_entries) VALUES (?, ?, ?, ?)`,
		raffleID, charName, world, numEntries)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// AddRaffleEntries adds entries to an existing raffle_entries row.
func (s *Store) AddRaffleEntries(entryID int64, additionalEntries int) error {
	_, err := s.db.Exec(`UPDATE raffle_entries SET num_entries = num_entries + ? WHERE id = ?`, additionalEntries, entryID)
	return err
}

// SetRaffleEntryPaid sets the paid flag on a raffle entry.
func (s *Store) SetRaffleEntryPaid(entryID int64, paid bool) error {
	val := 0
	if paid {
		val = 1
	}
	_, err := s.db.Exec(`UPDATE raffle_entries SET paid = ? WHERE id = ?`, val, entryID)
	return err
}

// SetRaffleWinner sets the winner_entry_id on a raffle.
func (s *Store) SetRaffleWinner(raffleID int64, entryID *int64) error {
	if entryID == nil {
		_, err := s.db.Exec(`UPDATE raffles SET winner_entry_id = NULL WHERE id = ?`, raffleID)
		return err
	}
	_, err := s.db.Exec(`UPDATE raffles SET winner_entry_id = ? WHERE id = ?`, *entryID, raffleID)
	return err
}

// SetRaffleStatus updates the raffle's status.
func (s *Store) SetRaffleStatus(raffleID int64, status string) error {
	_, err := s.db.Exec(`UPDATE raffles SET status = ? WHERE id = ?`, status, raffleID)
	return err
}

// PickRaffleWinner picks a random paid entry weighted by num_entries.
// Returns nil if no paid entries exist.
func (s *Store) PickRaffleWinner(raffleID int64) (*model.RaffleEntry, error) {
	// Get all paid entries
	rows, err := s.db.Query(`SELECT id, raffle_id, character_name, world, num_entries, paid, created_at FROM raffle_entries WHERE raffle_id = ? AND paid = 1`, raffleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.RaffleEntry
	totalTickets := 0
	for rows.Next() {
		var e model.RaffleEntry
		var paid int
		if err := rows.Scan(&e.ID, &e.RaffleID, &e.CharacterName, &e.World, &e.NumEntries, &paid, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Paid = paid != 0
		entries = append(entries, e)
		totalTickets += e.NumEntries
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(entries) == 0 || totalTickets == 0 {
		return nil, nil
	}

	// Weighted random pick
	pick := randInt(totalTickets)
	cumulative := 0
	for _, e := range entries {
		cumulative += e.NumEntries
		if pick < cumulative {
			return &e, nil
		}
	}
	// Fallback (shouldn't reach)
	return &entries[len(entries)-1], nil
}

// DeleteRaffleEntry removes a raffle entry by ID.
func (s *Store) DeleteRaffleEntry(entryID int64) (bool, error) {
	res, err := s.db.Exec("DELETE FROM raffle_entries WHERE id = ?", entryID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
