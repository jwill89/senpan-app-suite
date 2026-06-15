package store

import (
	"database/sql"
	"errors"
)

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
