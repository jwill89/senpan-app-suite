package store

import (
	"context"
	"database/sql"

	"app-suite/internal/model"
	"app-suite/internal/raffles"
)

// raffleColumns is the shared SELECT list for the base raffles row, matching the
// scan order in scanRaffle. (listRafflesAdmin appends its own aggregate columns.)
// It is written against raffleFrom's aliases, so the two are always used together.
const raffleColumns = `r.id, r.title, r.description, r.rules, r.max_entries, r.signup_instructions,
	r.entry_mode, r.cost_per_entry, r.tier_costs, COALESCE(r.available_from, ''), COALESCE(r.available_to, ''), r.prize_image,
	r.pay_image, r.status, r.winner_entry_id,
	r.festival_map_id, COALESCE(fm.title, ''), r.occupant_id, COALESCE(fo.title, ''), r.created_at`

// raffleFrom is the FROM clause raffleColumns is written against: the raffle plus
// the two optional festival joins - the map it is filed under, and the stall
// occupant it is assigned to. Both are LEFT joins; most raffles belong to no
// festival and both columns are NULL.
const raffleFrom = `FROM raffles r
	LEFT JOIN festival_maps fm ON fm.id = r.festival_map_id
	LEFT JOIN festival_stall_occupants fo ON fo.id = r.occupant_id`

// decodeTierCosts reads the "custom" mode's per-ticket price ladder from the
// tier_costs TEXT column, via the shared JSON-array codec (jsonarray.go): a legacy
// empty string or a malformed value decodes to an empty ladder rather than nil.
func decodeTierCosts(raw string) []float64 { return decodeJSONArray[float64](raw) }

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
	r.EntryMode = raffles.NormalizeMode(r.EntryMode)
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

// FestivalRaffle is one raffle filed under a festival, with what the festival
// dashboard shows beside it: its ticket total, and its winner once one is set.
type FestivalRaffle struct {
	model.Raffle
	TotalEntries int
	// WinnerName/WinnerWorld are the winner entry's, "" when no winner is set.
	WinnerName  string
	WinnerWorld string
}

// ListFestivalRaffles returns every raffle filed under a festival map, newest
// first, with its ticket total and winner. Which of them a visitor may see is the
// caller's call, made against the clock as on the public raffle pages. Two
// queries whatever the number of raffles: the rows, then one tally over them all.
func (s *Store) ListFestivalRaffles(ctx context.Context, mapID int64) ([]FestivalRaffle, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT `+raffleColumns+` `+raffleFrom+`
		WHERE r.festival_map_id = ? ORDER BY r.created_at DESC, r.id DESC`, mapID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	raffles := make([]FestivalRaffle, 0)
	for rows.Next() {
		r, err := scanRaffle(rows)
		if err != nil {
			return nil, err
		}
		raffles = append(raffles, FestivalRaffle{Raffle: r})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(raffles) == 0 {
		return raffles, nil
	}

	type tally struct {
		total       int
		name, world string
	}
	tallies := make(map[int64]tally, len(raffles))
	trows, err := s.read.QueryContext(ctx, `SELECT r.id,
			COALESCE((SELECT SUM(e.num_entries) FROM raffle_entries e WHERE e.raffle_id = r.id), 0),
			COALESCE(w.character_name, ''), COALESCE(w.world, '')
		FROM raffles r LEFT JOIN raffle_entries w ON w.id = r.winner_entry_id
		WHERE r.festival_map_id = ?`, mapID)
	if err != nil {
		return nil, err
	}
	defer trows.Close()
	for trows.Next() {
		var id int64
		var t tally
		if err := trows.Scan(&id, &t.total, &t.name, &t.world); err != nil {
			return nil, err
		}
		tallies[id] = t
	}
	if err := trows.Err(); err != nil {
		return nil, err
	}
	for i := range raffles {
		t := tallies[raffles[i].ID]
		raffles[i].TotalEntries = t.total
		raffles[i].WinnerName, raffles[i].WinnerWorld = t.name, t.world
	}
	return raffles, nil
}

// FestivalRaffleEntriesFor returns one participant's own entries across a
// festival's raffles, keyed by raffle id. The match is the one entries merge on
// (AddOrCreateRaffleEntry): the whole name and the whole world, ignoring case -
// never a substring, unlike the public lookup, because this answers "what have I
// entered" rather than "how did I spell it". Rows carry no money, as in the
// lookup (see model.RaffleLookupEntry).
func (s *Store) FestivalRaffleEntriesFor(ctx context.Context, mapID int64, name, world string) (map[int64]model.RaffleLookupEntry, error) {
	rows, err := s.read.QueryContext(ctx, `SELECT e.raffle_id, e.character_name, e.world, e.num_entries, e.paid_entries
		FROM raffle_entries e JOIN raffles r ON r.id = e.raffle_id
		WHERE r.festival_map_id = ? AND LOWER(e.character_name) = LOWER(?) AND LOWER(e.world) = LOWER(?)`,
		mapID, name, world)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byRaffle := make(map[int64]model.RaffleLookupEntry)
	for rows.Next() {
		var raffleID int64
		var e model.RaffleLookupEntry
		if err := rows.Scan(&raffleID, &e.CharacterName, &e.World, &e.NumEntries, &e.PaidEntries); err != nil {
			return nil, err
		}
		e.PaymentState = raffles.PaymentState(e.NumEntries, e.PaidEntries)
		byRaffle[raffleID] = e
	}
	return byRaffle, rows.Err()
}
