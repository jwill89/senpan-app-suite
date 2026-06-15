package store

import (
	"database/sql"
	"errors"

	"app-suite/internal/model"
)

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
//
// In admin mode each raffle also carries two read-only aggregates used by the
// closed-raffle table: WinnerName (the verified winner entry's "Character @ World",
// joined from raffle_entries) and PaidTotal (the sum of paid tickets × cost_per_entry,
// the gil collected). The public list omits both.
func (s *Store) ListRaffles(adminMode bool) ([]model.Raffle, error) {
	if adminMode {
		return s.listRafflesAdmin()
	}
	// Public list: only open raffles currently inside their availability
	// window. Availability dates are stored as UTC (RFC-3339 with 'Z' for new
	// values, legacy naive strings treated as UTC); datetime() normalizes both
	// to a UTC timestamp so the comparison against datetime('now') (also UTC)
	// is timezone-correct — a raffle past its "available to" instant no longer
	// shows regardless of the admin's timezone.
	rows, err := s.db.Query(`SELECT id, title, description, rules, max_entries, signup_instructions, cost_per_entry, available_from, available_to, prize_image, status, winner_entry_id, created_at FROM raffles WHERE status = 'open' AND (available_from = '' OR datetime(available_from) <= datetime('now')) AND (available_to = '' OR datetime(available_to) >= datetime('now')) ORDER BY created_at DESC`)
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

// listRafflesAdmin returns every raffle with the closed-table aggregates joined in:
// the winner entry's name (when a winner is set) and the total gil collected from
// paid tickets. The paid-ticket sum comes from a correlated subquery so raffles
// with no paid entries still report 0.
func (s *Store) listRafflesAdmin() ([]model.Raffle, error) {
	rows, err := s.db.Query(`SELECT r.id, r.title, r.description, r.rules, r.max_entries, r.signup_instructions,
			r.cost_per_entry, r.available_from, r.available_to, r.prize_image, r.status, r.winner_entry_id, r.created_at,
			COALESCE(w.character_name, ''), COALESCE(w.world, ''),
			COALESCE((SELECT SUM(e.num_entries) FROM raffle_entries e WHERE e.raffle_id = r.id AND e.paid = 1), 0)
		FROM raffles r
		LEFT JOIN raffle_entries w ON w.id = r.winner_entry_id
		ORDER BY r.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	raffles := make([]model.Raffle, 0)
	for rows.Next() {
		var r model.Raffle
		var winnerID sql.NullInt64
		var winnerChar, winnerWorld string
		var paidTickets int
		if err := rows.Scan(&r.ID, &r.Title, &r.Description, &r.Rules, &r.MaxEntries, &r.SignupInstructions,
			&r.CostPerEntry, &r.AvailableFrom, &r.AvailableTo, &r.PrizeImage, &r.Status, &winnerID, &r.CreatedAt,
			&winnerChar, &winnerWorld, &paidTickets); err != nil {
			return nil, err
		}
		if winnerID.Valid {
			r.WinnerEntryID = &winnerID.Int64
			if winnerChar != "" {
				r.WinnerName = winnerChar + " @ " + winnerWorld
			}
		}
		r.PaidTotal = float64(paidTickets) * r.CostPerEntry
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
