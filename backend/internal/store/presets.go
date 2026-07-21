package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"app-suite/internal/model"
)

// ── Game preset operations ──────────────────────────────────────────────────

// ListGamePresets returns all game presets ordered by name.
func (s *Store) ListGamePresets() ([]model.GamePreset, error) {
	rows, err := s.db.Query("SELECT id, name, pattern_ids, game_details, auto_call, auto_interval, created_at FROM game_presets ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	presets := make([]model.GamePreset, 0)
	for rows.Next() {
		var p model.GamePreset
		var idsJSON string
		if err := rows.Scan(&p.ID, &p.Name, &idsJSON, &p.GameDetails, &p.Auto, &p.AutoInterval, &p.CreatedAt); err != nil {
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
		"SELECT id, name, pattern_ids, game_details, auto_call, auto_interval, created_at FROM game_presets WHERE id = ?", id,
	).Scan(&p.ID, &p.Name, &idsJSON, &p.GameDetails, &p.Auto, &p.AutoInterval, &p.CreatedAt)
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
func (s *Store) CreateGamePreset(name string, patternIDs []int64, gameDetails string, auto bool, autoInterval int) (int64, error) {
	idsJSON, err := json.Marshal(normalizePresetIDs(patternIDs))
	if err != nil {
		return 0, fmt.Errorf("marshal preset pattern ids: %w", err)
	}
	res, err := s.db.Exec(
		"INSERT INTO game_presets (name, pattern_ids, game_details, auto_call, auto_interval) VALUES (?, ?, ?, ?, ?)",
		name, string(idsJSON), gameDetails, auto, autoInterval,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateGamePreset updates a preset's name, pattern IDs, game details, and auto-draw settings.
func (s *Store) UpdateGamePreset(id int64, name string, patternIDs []int64, gameDetails string, auto bool, autoInterval int) error {
	idsJSON, err := json.Marshal(normalizePresetIDs(patternIDs))
	if err != nil {
		return fmt.Errorf("marshal preset pattern ids: %w", err)
	}
	_, err = s.db.Exec(
		"UPDATE game_presets SET name = ?, pattern_ids = ?, game_details = ?, auto_call = ?, auto_interval = ? WHERE id = ?",
		name, string(idsJSON), gameDetails, auto, autoInterval, id,
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
