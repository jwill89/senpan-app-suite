package store

import (
	"database/sql"
	"errors"

	"app-suite/internal/model"
)

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

// GetStyle retrieves a single style by ID (with CSS + flourishes). Returns nil if
// not found.
func (s *Store) GetStyle(id int64) (*model.Style, error) {
	var st model.Style
	err := s.db.QueryRow(
		"SELECT id, name, css_content, board_flourish, number_flourish, created_at FROM styles WHERE id = ?", id,
	).Scan(&st.ID, &st.Name, &st.CSSContent, &st.BoardFlourish, &st.NumberFlourish, &st.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// CreateStyle inserts a new style and returns its ID.
func (s *Store) CreateStyle(name, css, boardFlourish, numberFlourish string) (int64, error) {
	res, err := s.db.Exec(
		"INSERT INTO styles (name, css_content, board_flourish, number_flourish) VALUES (?, ?, ?, ?)",
		name, css, boardFlourish, numberFlourish,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateStyle updates a style's name, CSS content, and flourishes.
func (s *Store) UpdateStyle(id int64, name, css, boardFlourish, numberFlourish string) error {
	_, err := s.db.Exec(
		"UPDATE styles SET name = ?, css_content = ?, board_flourish = ?, number_flourish = ? WHERE id = ?",
		name, css, boardFlourish, numberFlourish, id,
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

// GetActiveStyle returns the currently active style (with CSS + flourishes), or
// nil when no active style is set or it no longer exists. Used by the active-CSS
// endpoint and the live style broadcast so clients also receive the flourishes.
func (s *Store) GetActiveStyle() (*model.Style, error) {
	idStr, err := s.GetSetting("active_style_id")
	if err != nil || idStr == "" {
		return nil, err
	}
	var st model.Style
	err = s.db.QueryRow(
		"SELECT id, name, css_content, board_flourish, number_flourish, created_at FROM styles WHERE id = ?", idStr,
	).Scan(&st.ID, &st.Name, &st.CSSContent, &st.BoardFlourish, &st.NumberFlourish, &st.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
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
