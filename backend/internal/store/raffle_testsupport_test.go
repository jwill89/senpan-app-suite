package store

import (
	"context"
	"database/sql"
	"errors"

	"app-suite/internal/model"
)

// Test-only raffle-entry helpers. Production always writes entries through the
// atomic, cap-enforcing AddOrCreateRaffleEntry; these lower-level primitives exist
// solely to set up and inspect entry rows in the store tests, so they live here
// (compiled only for `go test`) rather than on the shipped Store surface.

// GetRaffleEntry retrieves a single raffle entry by character+world for a raffle
// (case-insensitive), or nil when none matches.
func (s *Store) GetRaffleEntry(ctx context.Context, raffleID int64, charName, world string) (*model.RaffleEntry, error) {
	e, err := scanRaffleEntry(s.db.QueryRowContext(ctx, `SELECT `+raffleEntryColumns+` FROM raffle_entries WHERE raffle_id = ? AND LOWER(character_name) = LOWER(?) AND LOWER(world) = LOWER(?)`,
		raffleID, charName, world))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// CreateRaffleEntry inserts a new raffle entry (no cap enforcement).
func (s *Store) CreateRaffleEntry(ctx context.Context, raffleID int64, charName, world string, numEntries int) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO raffle_entries (raffle_id, character_name, world, num_entries) VALUES (?, ?, ?, ?)`,
		raffleID, charName, world, numEntries)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// AddRaffleEntries adds entries to an existing raffle_entries row.
func (s *Store) AddRaffleEntries(ctx context.Context, entryID int64, additionalEntries int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE raffle_entries SET num_entries = num_entries + ? WHERE id = ?`, additionalEntries, entryID)
	return err
}
