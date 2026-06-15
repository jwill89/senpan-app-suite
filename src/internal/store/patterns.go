package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"app-suite/internal/model"
)

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
