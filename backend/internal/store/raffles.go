package store

import (
	"database/sql"
	"errors"
	"strings"

	"app-suite/internal/model"
)

// ErrRaffleEntryLimit is returned by AddOrCreateRaffleEntry when recording the
// requested entries would push a character+world over the raffle's per-player cap.
var ErrRaffleEntryLimit = errors.New("raffle entry limit exceeded")

// -- Raffle operations -------------------------------------------------------

// CreateRaffle inserts a new raffle and returns its ID.
func (s *Store) CreateRaffle(r *model.Raffle) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO raffles (title, description, rules, max_entries, signup_instructions, entry_mode, cost_per_entry, tier_costs, available_from, available_to, prize_image, pay_image, status, festival_map_id, occupant_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Title, r.Description, r.Rules, r.MaxEntries, r.SignupInstructions,
		r.Mode(), r.CostPerEntry, encodeTierCosts(r.TierCosts),
		r.AvailableFrom, r.AvailableTo, r.PrizeImage, r.PayImage, "open",
		nullableID(r.FestivalMapID), nullableID(r.OccupantID),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateRaffle updates an existing raffle's editable fields.
func (s *Store) UpdateRaffle(r *model.Raffle) error {
	_, err := s.db.Exec(`UPDATE raffles SET title=?, description=?, rules=?, max_entries=?, signup_instructions=?, entry_mode=?, cost_per_entry=?, tier_costs=?, available_from=?, available_to=?, prize_image=?, pay_image=?, festival_map_id=?, occupant_id=? WHERE id=?`,
		r.Title, r.Description, r.Rules, r.MaxEntries, r.SignupInstructions,
		r.Mode(), r.CostPerEntry, encodeTierCosts(r.TierCosts),
		r.AvailableFrom, r.AvailableTo, r.PrizeImage, r.PayImage,
		nullableID(r.FestivalMapID), nullableID(r.OccupantID), r.ID,
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

// raffleColumns is the shared SELECT list for the base raffles row, matching the
// scan order in scanRaffle. (listRafflesAdmin appends its own aggregate columns.)
// It is written against raffleFrom's aliases, so the two are always used together.
const raffleColumns = `r.id, r.title, r.description, r.rules, r.max_entries, r.signup_instructions,
	r.entry_mode, r.cost_per_entry, r.tier_costs, r.available_from, r.available_to, r.prize_image,
	r.pay_image, r.status, r.winner_entry_id,
	r.festival_map_id, COALESCE(fm.title, ''), r.occupant_id, COALESCE(fo.title, ''), r.created_at`

// raffleFrom is the FROM clause raffleColumns is written against: the raffle plus
// the two optional festival joins - the map it is filed under, and the stall
// occupant it is assigned to. Both are LEFT joins; most raffles belong to no
// festival and both columns are NULL.
const raffleFrom = `FROM raffles r
	LEFT JOIN festival_maps fm ON fm.id = r.festival_map_id
	LEFT JOIN festival_stall_occupants fo ON fo.id = r.occupant_id`

// encodeTierCosts / decodeTierCosts persist the "custom" mode's per-ticket price
// ladder in the tier_costs TEXT column, via the shared JSON-array codecs
// (jsonarray.go) - so a legacy empty string or a malformed value decodes to an
// empty ladder rather than nil.
func encodeTierCosts(costs []float64) string { return encodeJSONArray(costs) }
func decodeTierCosts(raw string) []float64   { return decodeJSONArray[float64](raw) }

// scanRaffle scans one base raffles row (winner_entry_id nullable). Shared by
// GetRaffle and the public ListRaffles so the column order lives in one place.
func scanRaffle(sc rowScanner) (model.Raffle, error) {
	var r model.Raffle
	var winnerID, mapID, occupantID sql.NullInt64
	var tierJSON string
	if err := sc.Scan(&r.ID, &r.Title, &r.Description, &r.Rules, &r.MaxEntries, &r.SignupInstructions,
		&r.EntryMode, &r.CostPerEntry, &tierJSON, &r.AvailableFrom, &r.AvailableTo, &r.PrizeImage,
		&r.PayImage, &r.Status, &winnerID,
		&mapID, &r.FestivalMapName, &occupantID, &r.StallName, &r.CreatedAt); err != nil {
		return r, err
	}
	r.EntryMode = model.NormalizeRaffleMode(r.EntryMode)
	r.TierCosts = decodeTierCosts(tierJSON)
	if winnerID.Valid {
		r.WinnerEntryID = &winnerID.Int64
	}
	if mapID.Valid {
		id := mapID.Int64
		r.FestivalMapID = &id
	}
	if occupantID.Valid {
		id := occupantID.Int64
		r.OccupantID = &id
	}
	return r, nil
}

// GetRaffle retrieves a single raffle by ID. Returns nil if not found.
func (s *Store) GetRaffle(id int64) (*model.Raffle, error) {
	r, err := scanRaffle(s.db.QueryRow(`SELECT ` + raffleColumns + ` ` + raffleFrom + ` WHERE r.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListRaffles returns raffles. If adminMode is false, returns open raffles within the
// availability date range plus all closed raffles (for browsing past results).
//
// In admin mode each raffle also carries two read-only aggregates used by the
// closed-raffle table: WinnerName (the verified winner entry's "Character @ World",
// joined from raffle_entries) and PaidTotal (the sum of paid tickets x cost_per_entry,
// the gil collected). The public list omits both.
func (s *Store) ListRaffles(adminMode bool) ([]model.Raffle, error) {
	if adminMode {
		return s.listRafflesAdmin()
	}
	// Public list: only open raffles currently inside their availability
	// window. Availability dates are stored as UTC (RFC-3339 with 'Z' for new
	// values, legacy naive strings treated as UTC); datetime() normalizes both
	// to a UTC timestamp so the comparison against datetime('now') (also UTC)
	// is timezone-correct - a raffle past its "available to" instant no longer
	// shows regardless of the admin's timezone.
	rows, err := s.db.Query(`SELECT ` + raffleColumns + ` ` + raffleFrom + `
		WHERE r.status = 'open'
		  AND (r.available_from = '' OR datetime(r.available_from) <= datetime('now'))
		  AND (r.available_to = '' OR datetime(r.available_to) >= datetime('now'))
		ORDER BY r.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	raffles := make([]model.Raffle, 0)
	for rows.Next() {
		r, err := scanRaffle(rows)
		if err != nil {
			return nil, err
		}
		raffles = append(raffles, r)
	}
	return raffles, rows.Err()
}

// listRafflesAdmin returns every raffle with the closed-table aggregates joined
// in: the winner entry's name (when a winner is set) and the total gil collected
// from paid tickets.
//
// The paid total can't be a SQL SUM any more: a "custom" raffle prices its 1st,
// 2nd and 3rd ticket differently, so what an entry owes depends on how many
// tickets it holds, and each entry may have had part of that waived. Instead the
// settlement of every entry is read and priced through Raffle.AmountCollected,
// which knows each mode's rule. That is three small queries over an admin-only
// list rather than one, and it keeps the pricing rule in exactly one place.
func (s *Store) listRafflesAdmin() ([]model.Raffle, error) {
	rows, err := s.db.Query(`SELECT ` + raffleColumns + ` ` + raffleFrom + ` ORDER BY r.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	raffles := make([]model.Raffle, 0)
	for rows.Next() {
		r, err := scanRaffle(rows)
		if err != nil {
			return nil, err
		}
		raffles = append(raffles, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	winners, err := s.raffleWinnerNames()
	if err != nil {
		return nil, err
	}
	settled, err := s.settledRaffleEntries()
	if err != nil {
		return nil, err
	}
	for i := range raffles {
		r := &raffles[i]
		if r.WinnerEntryID != nil {
			r.WinnerName = winners[*r.WinnerEntryID]
		}
		for _, e := range settled[r.ID] {
			r.PaidTotal += r.AmountCollected(e)
		}
	}
	return raffles, nil
}

// raffleWinnerNames maps a winner entry id to its "Character @ World" label, for
// every raffle that has one set.
func (s *Store) raffleWinnerNames() (map[int64]string, error) {
	rows, err := s.db.Query(`SELECT e.id, e.character_name, e.world FROM raffle_entries e
		JOIN raffles r ON r.winner_entry_id = e.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names := make(map[int64]string)
	for rows.Next() {
		var id int64
		var char, world string
		if err := rows.Scan(&id, &char, &world); err != nil {
			return nil, err
		}
		if char != "" {
			names[id] = char + " @ " + world
		}
	}
	return names, rows.Err()
}

// settledRaffleEntries maps a raffle id to the settlement of each of its entries
// that has paid for at least one ticket. Kept per entry rather than summed,
// because a tiered raffle prices "one entry holding 3 tickets" differently from
// "three entries holding 1", and each entry carries its own waived amount.
// Partly-settled entries are included: they really did hand gil over.
func (s *Store) settledRaffleEntries() (map[int64][]model.RaffleEntry, error) {
	rows, err := s.db.Query(`SELECT raffle_id, paid_entries, amount_waived FROM raffle_entries WHERE paid_entries > 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byRaffle := make(map[int64][]model.RaffleEntry)
	for rows.Next() {
		var e model.RaffleEntry
		if err := rows.Scan(&e.RaffleID, &e.PaidEntries, &e.AmountWaived); err != nil {
			return nil, err
		}
		byRaffle[e.RaffleID] = append(byRaffle[e.RaffleID], e)
	}
	return byRaffle, rows.Err()
}

// CountRaffleEntries returns the total number of entries (sum of num_entries) for a raffle.
func (s *Store) CountRaffleEntries(raffleID int64) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COALESCE(SUM(num_entries), 0) FROM raffle_entries WHERE raffle_id = ?`, raffleID).Scan(&count)
	return count, err
}

// raffleEntryColumns is the shared SELECT list for raffle_entries, matching the
// scan order in scanRaffleEntry.
const raffleEntryColumns = "id, raffle_id, character_name, world, num_entries, paid_entries, amount_waived, paid, created_at"

// scanRaffleEntry scans one raffle_entries row (paid stored as 0/1). Shared by the
// list/get/pick paths so the column order + paid conversion live in one place.
func scanRaffleEntry(sc rowScanner) (model.RaffleEntry, error) {
	var e model.RaffleEntry
	var paid int
	if err := sc.Scan(&e.ID, &e.RaffleID, &e.CharacterName, &e.World, &e.NumEntries,
		&e.PaidEntries, &e.AmountWaived, &paid, &e.CreatedAt); err != nil {
		return e, err
	}
	e.Paid = paid != 0
	return e, nil
}

// ListRaffleEntries returns all entries for a raffle.
func (s *Store) ListRaffleEntries(raffleID int64) ([]model.RaffleEntry, error) {
	rows, err := s.db.Query(`SELECT `+raffleEntryColumns+` FROM raffle_entries WHERE raffle_id = ? ORDER BY created_at ASC`, raffleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]model.RaffleEntry, 0)
	for rows.Next() {
		e, err := scanRaffleEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetRaffleEntryByID returns a single raffle entry by its ID.
func (s *Store) GetRaffleEntryByID(entryID int64) (*model.RaffleEntry, error) {
	e, err := scanRaffleEntry(s.db.QueryRow(`SELECT `+raffleEntryColumns+` FROM raffle_entries WHERE id = ?`, entryID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// AddOrCreateRaffleEntry records num additional entries for a character+world on a
// raffle, enforcing the per-player cap (maxEntries) inside one write transaction so
// concurrent public sign-ups can neither exceed the cap nor create duplicate rows
// for the same character+world. It updates the existing row if one exists (matched
// case-insensitively on character_name + world) or inserts a new one otherwise.
//
// Returns the affected row's id, the running total after the change, the count that
// existed before it, and whether a new row was created. When the cap would be
// exceeded it makes no change and returns ErrRaffleEntryLimit (with prevEntries set
// so the caller can phrase the message). Callers should still pre-validate raffle
// status/availability; this method only owns the atomic cap-enforced write.
func (s *Store) AddOrCreateRaffleEntry(raffleID int64, charName, world string, num, maxEntries int) (entryID int64, newTotal, prevEntries int, created bool, err error) {
	tx, err := s.beginImmediate()
	if err != nil {
		return 0, 0, 0, false, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRow(`SELECT id, num_entries FROM raffle_entries
		WHERE raffle_id = ? AND LOWER(character_name) = LOWER(?) AND LOWER(world) = LOWER(?)`,
		raffleID, charName, world)
	switch scanErr := row.Scan(&entryID, &prevEntries); {
	case errors.Is(scanErr, sql.ErrNoRows):
		created = true
	case scanErr != nil:
		return 0, 0, 0, false, scanErr
	}

	// Test the cap by subtraction, never by summing first: num arrives from a
	// request body, and prevEntries+num on a huge num wraps negative, sails past
	// a "> maxEntries" check, and writes an overflowed count SQLite has to widen
	// to REAL - a value no read path can scan back into an int, which strands the
	// whole raffle. Subtracting cannot overflow here because both operands are
	// already in range.
	if num > maxEntries-prevEntries {
		return 0, 0, prevEntries, created, ErrRaffleEntryLimit
	}
	newTotal = prevEntries + num

	if created {
		res, err := tx.Exec(`INSERT INTO raffle_entries (raffle_id, character_name, world, num_entries) VALUES (?, ?, ?, ?)`,
			raffleID, charName, world, num)
		if err != nil {
			return 0, 0, 0, false, err
		}
		if entryID, err = res.LastInsertId(); err != nil {
			return 0, 0, 0, false, err
		}
	} else {
		// Adding tickets to an already-settled row leaves the new ones unpaid, so
		// the derived paid flag has to drop back off. paid_entries and
		// amount_waived stay put - what was settled and forgiven still was - and
		// the row now reads as PARTIAL until someone records the difference.
		if _, err := tx.Exec(`UPDATE raffle_entries
			SET num_entries = num_entries + ?,
			    paid = CASE WHEN paid_entries >= num_entries + ? THEN 1 ELSE 0 END
			WHERE id = ?`, num, num, entryID); err != nil {
			return 0, 0, 0, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, 0, false, err
	}
	return entryID, newTotal, prevEntries, created, nil
}

// SetRaffleEntryPaid records a settlement on a raffle entry.
//
// Settling (paid=true) marks EVERY ticket on the row as settled and ADDS waived
// to whatever has already been forgiven - it never overwrites it, so a player who
// had one entry waived and later buys two more keeps both waivers. Clearing
// (paid=false) resets the row to untouched: nothing settled, nothing waived, so
// a mis-click doesn't leave a waiver credited against an unpaid entry.
//
// tickets says how many of the entry's tickets this settlement covers: 0 (or
// anything past the ticket count) means "all of them", which is the common
// counter case. A smaller number records a PART payment - somebody handing over
// gil for two of their three entries now and the rest later - and leaves the row
// reading as partial.
//
// Settling only ever moves FORWARD (`WHERE ... > paid_entries`), which makes it
// IDEMPOTENT: two staff settling the same entry at once, or one double-submit,
// can't stack the same waiver twice and quietly understate what the raffle
// collected, and a stale count can't walk a settlement backwards. The guard is
// part of the UPDATE rather than a read-then-write, which would race exactly the
// same way. To undo, clear the entry (paid=false) and record it again. A no-op
// reports itself through applied=false; the caller re-reads the row either way,
// so what the admin sees is always the settlement that stuck.
func (s *Store) SetRaffleEntryPaid(entryID int64, paid bool, tickets int, waived float64) (applied bool, err error) {
	if !paid {
		res, err := s.db.Exec(`UPDATE raffle_entries SET paid = 0, paid_entries = 0, amount_waived = 0 WHERE id = ?`, entryID)
		if err != nil {
			return false, err
		}
		n, _ := res.RowsAffected()
		return n > 0, nil
	}

	var res sql.Result
	if tickets <= 0 {
		res, err = s.db.Exec(`UPDATE raffle_entries
			SET paid = 1, paid_entries = num_entries, amount_waived = amount_waived + ?
			WHERE id = ? AND paid_entries < num_entries`, waived, entryID)
	} else {
		// MIN() in SQL, not a clamp in Go: num_entries can grow between the read and
		// the write (a player buying more tickets), and only the statement itself
		// sees the row's real count.
		res, err = s.db.Exec(`UPDATE raffle_entries
			SET paid_entries = MIN(?, num_entries),
			    paid = CASE WHEN MIN(?, num_entries) >= num_entries THEN 1 ELSE 0 END,
			    amount_waived = amount_waived + ?
			WHERE id = ? AND MIN(?, num_entries) > paid_entries`,
			tickets, tickets, waived, entryID, tickets)
	}
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
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

// PickRaffleWinner picks a random entry weighted by its eligible tickets, and
// returns nil when there is nothing to pick from.
//
// paidOnly is what a raffle that charges for tickets wants: an entry gets one
// chance per ticket it has SETTLED, not per ticket it holds. That matters because
// entries merge - somebody who paid for one ticket and then bought two more is
// part-paid, and neither dropping them from the draw nor giving them all three
// chances is honest. A fully-settled entry has paid_entries == num_entries, so
// this is the same draw it always was for them.
//
// A details-only raffle collects nothing through the app - its entries are
// recorded by staff from a sign-up that happened elsewhere - so there every entry
// is eligible for all of its tickets and the paid flag is ignored.
func (s *Store) PickRaffleWinner(raffleID int64, paidOnly bool) (*model.RaffleEntry, error) {
	where := `WHERE raffle_id = ?`
	if paidOnly {
		where += ` AND paid_entries > 0`
	}
	rows, err := s.db.Query(`SELECT `+raffleEntryColumns+` FROM raffle_entries `+where, raffleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Weight per entry, kept alongside it so the cumulative walk below can't drift
	// from the total it was drawn against.
	var entries []model.RaffleEntry
	var weights []int
	totalTickets := 0
	for rows.Next() {
		e, err := scanRaffleEntry(rows)
		if err != nil {
			return nil, err
		}
		tickets := e.NumEntries
		if paidOnly {
			tickets = min(e.PaidEntries, e.NumEntries)
		}
		if tickets < 1 {
			continue
		}
		entries = append(entries, e)
		weights = append(weights, tickets)
		totalTickets += tickets
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if totalTickets == 0 {
		return nil, nil
	}

	// Weighted random pick
	pick := randInt(totalTickets)
	cumulative := 0
	for i, e := range entries {
		cumulative += weights[i]
		if pick < cumulative {
			return &e, nil
		}
	}
	// Fallback (shouldn't reach)
	return &entries[len(entries)-1], nil
}

// DeleteRaffleEntry removes a raffle entry by ID.
func (s *Store) DeleteRaffleEntry(entryID int64) (bool, error) {
	tx, err := s.beginImmediate()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	// raffles.winner_entry_id is a plain column, not an enforced FK, so deleting the
	// row it points at left the raffle naming an entry that no longer exists -
	// and verify-winner then closed the raffle announcing nobody. Clear the pointer
	// in the same transaction so the raffle falls back to "no winner picked yet",
	// which is recoverable: staff can simply pick again.
	if _, err := tx.Exec(`UPDATE raffles SET winner_entry_id = NULL WHERE winner_entry_id = ?`, entryID); err != nil {
		return false, err
	}
	res, err := tx.Exec("DELETE FROM raffle_entries WHERE id = ?", entryID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return n > 0, nil
}

// escapeLikePattern makes a user-typed string safe to drop inside a LIKE
// pattern. Without it a search for "_" or "%" is a wildcard that matches every
// entrant instead of the literal character they typed. Pair it with ESCAPE '\'.
func escapeLikePattern(q string) string {
	// The escape character has to be escaped FIRST and with itself doubled -
	// replacing a backslash with a backslash was a no-op, so a typed backslash
	// stayed live: "\%" reached SQLite as an escaped percent (matching a literal %
	// rather than the two characters typed), and a trailing backslash left a
	// dangling escape. NewReplacer scans left to right and never re-processes what
	// it emits, so listing the backslash pair first is safe.
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(q)
}

// LookupRaffleEntries finds a raffle's entries whose character name CONTAINS the
// query (case-insensitive). This is the public "have I already entered?" search,
// whose job is to hand somebody back the exact spelling they signed up with -
// hence a substring match rather than the whole-string match the stamp-rally
// lookup uses, which exists to keep participation private.
//
// It returns at most limit rows plus a truncated flag, so a broad query reports
// itself as incomplete rather than passing a clipped list off as the whole set.
// Rows carry no entry id and no money (see model.RaffleLookupEntry).
func (s *Store) LookupRaffleEntries(raffleID int64, query string, limit int) (entries []model.RaffleLookupEntry, truncated bool, err error) {
	if limit < 1 {
		limit = 1
	}
	// Fetch one more than asked for: if it comes back, there are further matches.
	rows, err := s.db.Query(`SELECT character_name, world, num_entries, paid_entries
		FROM raffle_entries
		WHERE raffle_id = ? AND character_name LIKE ? ESCAPE '\' COLLATE NOCASE
		ORDER BY character_name COLLATE NOCASE, world COLLATE NOCASE
		LIMIT ?`, raffleID, "%"+escapeLikePattern(query)+"%", limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	entries = make([]model.RaffleLookupEntry, 0, limit)
	for rows.Next() {
		var e model.RaffleLookupEntry
		if err := rows.Scan(&e.CharacterName, &e.World, &e.NumEntries, &e.PaidEntries); err != nil {
			return nil, false, err
		}
		if len(entries) == limit {
			truncated = true
			break
		}
		e.PaymentState = model.RaffleEntry{NumEntries: e.NumEntries, PaidEntries: e.PaidEntries}.PaymentState()
		entries = append(entries, e)
	}
	return entries, truncated, rows.Err()
}
