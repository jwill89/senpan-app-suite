package store

import (
	"database/sql"
	"errors"

	"app-suite/internal/model"
)

// -- Festival Maps ------------------------------------------------------------
//
// A Festival Map (see model.FestivalMap) is an event owning stalls, each drawn on
// the base map image at a %-based placement. This layer is pure data access; the
// publish/availability logic (which maps the public may see, whether a stall is
// open right now) lives in server/festivalmaps.go, mirroring how the stamp rally
// keeps its time-window checks in the server layer.
//
// The map's and each stall's datetime ranges are stored as JSON arrays (like
// affiliates.hours) rather than sub-tables: they are small, always loaded with
// their row, and edited as a set.

// ErrStallNotOnMap is returned by UpdateFestivalMap when a save carries a stall id
// that does not belong to the map being saved - a stale client or a spoofed id.
// The whole save is rejected rather than partially applied, because reconciling
// that stall's occupants would edit a DIFFERENT map's pitch.
var ErrStallNotOnMap = errors.New("stall does not belong to this map")

// encodeEventTimes marshals a datetime-range list for its TEXT column.
func encodeEventTimes(times []model.EventTime) string { return encodeJSONArray(times) }

// decodeEventTimes reads a datetime-range list back, never returning nil.
func decodeEventTimes(raw string) []model.EventTime {
	return decodeJSONArray[model.EventTime](raw)
}

// -- Maps ---------------------------------------------------------------------

// ListFestivalMaps returns every map, newest first, each with its stall count.
// Stalls themselves are omitted (use GetFestivalMap).
func (s *Store) ListFestivalMaps() ([]model.FestivalMap, error) {
	rows, err := s.db.Query(`SELECT m.id, m.title, m.slug, m.description, m.times, m.map_image, m.status, m.created_at,
			COALESCE((SELECT COUNT(*) FROM festival_stalls st WHERE st.map_id = m.id), 0)
		FROM festival_maps m ORDER BY m.created_at DESC, m.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	maps := make([]model.FestivalMap, 0)
	for rows.Next() {
		var m model.FestivalMap
		var times string
		if err := rows.Scan(&m.ID, &m.Title, &m.Slug, &m.Description, &times, &m.MapImage, &m.Status,
			&m.CreatedAt, &m.StallCount); err != nil {
			return nil, err
		}
		m.Times = decodeEventTimes(times)
		m.Status = model.NormalizeMapStatus(m.Status)
		maps = append(maps, m)
	}
	return maps, rows.Err()
}

// GetFestivalMap returns a single map with its stalls (affiliate name joined).
// Returns nil if not found.
func (s *Store) GetFestivalMap(id int64) (*model.FestivalMap, error) {
	return s.getFestivalMapWhere(`id = ?`, id)
}

// GetFestivalMapBySlug returns the map carrying a shortcode, or nil if none does.
// The empty slug is never matched - it means "no shortcode", which most maps
// share, so looking one up would return an arbitrary map rather than nothing.
func (s *Store) GetFestivalMapBySlug(slug string) (*model.FestivalMap, error) {
	if slug == "" {
		return nil, nil
	}
	return s.getFestivalMapWhere(`slug = ?`, slug)
}

// getFestivalMapWhere loads one map (with its stalls) by an arbitrary single-arg
// predicate, shared by the id and slug lookups. Returns nil if no row matches.
func (s *Store) getFestivalMapWhere(where string, arg any) (*model.FestivalMap, error) {
	var m model.FestivalMap
	var times string
	err := s.db.QueryRow(`SELECT id, title, slug, description, times, map_image, status, created_at
		FROM festival_maps WHERE `+where, arg).
		Scan(&m.ID, &m.Title, &m.Slug, &m.Description, &times, &m.MapImage, &m.Status, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.Times = decodeEventTimes(times)
	m.Status = model.NormalizeMapStatus(m.Status)
	stalls, err := s.ListFestivalStalls(m.ID)
	if err != nil {
		return nil, err
	}
	m.Stalls = stalls
	return &m, nil
}

// ListFestivalStalls loads a map's pitches in display order, each with the
// occupants standing in it (affiliate name joined; empty means the venue's own
// booth, resolved for display on the frontend). Occupants are fetched for the
// whole map in one query rather than per pitch.
func (s *Store) ListFestivalStalls(mapID int64) ([]model.FestivalStall, error) {
	rows, err := s.db.Query(`SELECT id, map_id, shape, color, selection_color, text_color,
			pos_x, pos_y, width, height, rotation, sort_order
		FROM festival_stalls WHERE map_id = ? ORDER BY sort_order ASC, id ASC`, mapID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stalls := make([]model.FestivalStall, 0)
	for rows.Next() {
		var st model.FestivalStall
		if err := rows.Scan(&st.ID, &st.MapID, &st.Shape, &st.Color, &st.SelectionColor, &st.TextColor,
			&st.X, &st.Y, &st.Width, &st.Height, &st.Rotation, &st.SortOrder); err != nil {
			return nil, err
		}
		st.Shape = model.NormalizeStallShape(st.Shape)
		st.Occupants = []model.FestivalStallOccupant{}
		stalls = append(stalls, st)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	byStall, err := s.listOccupantsForMap(mapID)
	if err != nil {
		return nil, err
	}
	for i := range stalls {
		if occupants, found := byStall[stalls[i].ID]; found {
			stalls[i].Occupants = occupants
		}
	}
	return stalls, nil
}

// listOccupantsForMap loads every occupant on a map's pitches in display order,
// keyed by pitch id - one query for the whole plan rather than one per pitch.
func (s *Store) listOccupantsForMap(mapID int64) (map[int64][]model.FestivalStallOccupant, error) {
	rows, err := s.db.Query(`SELECT o.id, o.stall_id, o.affiliate_id, COALESCE(a.name, ''),
			o.title, o.description, o.event_carrd, o.stall_type, o.type_label, o.times, o.sort_order
		FROM festival_stall_occupants o
		JOIN festival_stalls st ON st.id = o.stall_id
		LEFT JOIN affiliates a ON a.id = o.affiliate_id
		WHERE st.map_id = ? ORDER BY o.sort_order ASC, o.id ASC`, mapID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byStall := make(map[int64][]model.FestivalStallOccupant)
	for rows.Next() {
		var o model.FestivalStallOccupant
		var affiliateID sql.NullInt64
		var times string
		if err := rows.Scan(&o.ID, &o.StallID, &affiliateID, &o.AffiliateName,
			&o.Title, &o.Description, &o.EventCarrd, &o.StallType, &o.TypeLabel,
			&times, &o.SortOrder); err != nil {
			return nil, err
		}
		if affiliateID.Valid {
			id := affiliateID.Int64
			o.AffiliateID = &id
		}
		o.Times = decodeEventTimes(times)
		o.StallType = model.NormalizeStallType(o.StallType)
		byStall[o.StallID] = append(byStall[o.StallID], o)
	}
	return byStall, rows.Err()
}

// CreateFestivalMap inserts a new map and its stalls in one transaction, returning
// the new map's ID.
func (s *Store) CreateFestivalMap(m *model.FestivalMap) (int64, error) {
	tx, err := s.beginImmediate()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`INSERT INTO festival_maps (title, slug, description, times, map_image, status)
		VALUES (?, ?, ?, ?, ?, ?)`,
		m.Title, m.Slug, m.Description, encodeEventTimes(m.Times), m.MapImage, m.Status)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, st := range m.Stalls {
		if _, err := insertFestivalStall(tx, id, st, i); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// UpdateFestivalMap updates a map's editable fields and reconciles its stalls in
// one transaction. Stalls are UPSERTED by id (existing ids updated, id==0
// inserted, omitted ids deleted) so a Stamp Rally stamp that names a stall keeps
// naming it across map edits; a deleted stall's stamps fall back to their
// affiliate (stall_id is cleared, not cascaded, since it is an unenforced link).
// Status is not touched here - it is set through SetFestivalMapStatus.
// replaceStalls says whether this save is authoritative for the map's pitches.
// False leaves them exactly as they are - see the note on UpdateStampRally about
// why "the request didn't mention them" and "the request wants none" have to be
// different answers.
func (s *Store) UpdateFestivalMap(m *model.FestivalMap, replaceStalls bool) error {
	tx, err := s.beginImmediate()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`UPDATE festival_maps SET title = ?, slug = ?, description = ?, times = ?, map_image = ?
		WHERE id = ?`,
		m.Title, m.Slug, m.Description, encodeEventTimes(m.Times), m.MapImage, m.ID); err != nil {
		return err
	}

	if !replaceStalls {
		return tx.Commit()
	}

	keep := make(map[int64]bool, len(m.Stalls))
	for i, st := range m.Stalls {
		if st.ID > 0 {
			if err := updateFestivalStall(tx, m.ID, st, i); err != nil {
				return err
			}
			keep[st.ID] = true
			continue
		}
		newID, err := insertFestivalStall(tx, m.ID, st, i)
		if err != nil {
			return err
		}
		keep[newID] = true
	}

	existing, err := tx.Query(`SELECT id FROM festival_stalls WHERE map_id = ?`, m.ID)
	if err != nil {
		return err
	}
	var toDelete []int64
	for existing.Next() {
		var id int64
		if err := existing.Scan(&id); err != nil {
			existing.Close()
			return err
		}
		if !keep[id] {
			toDelete = append(toDelete, id)
		}
	}
	existing.Close()
	if err := existing.Err(); err != nil {
		return err
	}
	for _, id := range toDelete {
		if _, err := tx.Exec(`UPDATE stamp_rally_stamps SET occupant_id = NULL WHERE occupant_id IN
			(SELECT id FROM festival_stall_occupants WHERE stall_id = ?)`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE raffles SET occupant_id = NULL WHERE occupant_id IN
			(SELECT id FROM festival_stall_occupants WHERE stall_id = ?)`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM festival_stalls WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// insertFestivalStall inserts one pitch at the given sort order along with its
// occupants, returning its ID.
func insertFestivalStall(tx *sql.Tx, mapID int64, st model.FestivalStall, sortOrder int) (int64, error) {
	res, err := tx.Exec(`INSERT INTO festival_stalls
			(map_id, shape, color, selection_color, text_color, pos_x, pos_y, width, height, rotation, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		mapID, st.Shape, st.Color, st.SelectionColor, st.TextColor,
		st.X, st.Y, st.Width, st.Height, st.Rotation, sortOrder)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, replaceStallOccupants(tx, id, st.Occupants)
}

// updateFestivalStall updates an existing pitch's drawn form + sort order and
// reconciles its occupants. The WHERE clause is scoped to mapID as well as the
// stall id so a spoofed stall id belonging to a different map can't be updated
// across maps - and the row count is CHECKED, because the occupant reconciliation
// below is keyed on the stall id alone. Scoping only the UPDATE left the guard
// half-applied: a save of map A carrying a stall id from map B matched no row
// here (correctly), then went on to rewrite and delete map B's occupants for that
// pitch, clearing the rally stamps and raffles that named them. A zero row count
// also catches the benign race where a concurrent save deleted the stall, which
// otherwise resurrects its occupants as orphans.
func updateFestivalStall(tx *sql.Tx, mapID int64, st model.FestivalStall, sortOrder int) error {
	res, err := tx.Exec(`UPDATE festival_stalls SET shape = ?, color = ?, selection_color = ?, text_color = ?,
			pos_x = ?, pos_y = ?, width = ?, height = ?, rotation = ?, sort_order = ?
		WHERE id = ? AND map_id = ?`,
		st.Shape, st.Color, st.SelectionColor, st.TextColor,
		st.X, st.Y, st.Width, st.Height, st.Rotation, sortOrder,
		st.ID, mapID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrStallNotOnMap
	}
	return replaceStallOccupants(tx, st.ID, st.Occupants)
}

// replaceStallOccupants reconciles a pitch's occupants BY ID - existing ids
// updated, id==0 inserted, omitted ids deleted - rather than replacing them
// wholesale. A stamp rally names occupants, so an occupant that survives an edit
// has to keep the id its stamps point at.
func replaceStallOccupants(tx *sql.Tx, stallID int64, occupants []model.FestivalStallOccupant) error {
	keep := make(map[int64]bool, len(occupants))
	for i, o := range occupants {
		if o.ID > 0 {
			if _, err := tx.Exec(`UPDATE festival_stall_occupants
					SET affiliate_id = ?, title = ?, description = ?, event_carrd = ?,
						stall_type = ?, type_label = ?, times = ?, sort_order = ?
					WHERE id = ? AND stall_id = ?`,
				nullableID(o.AffiliateID), o.Title, o.Description, o.EventCarrd,
				o.StallType, o.TypeLabel, encodeEventTimes(o.Times), i, o.ID, stallID); err != nil {
				return err
			}
			keep[o.ID] = true
			continue
		}
		res, err := tx.Exec(`INSERT INTO festival_stall_occupants
				(stall_id, affiliate_id, title, description, event_carrd, stall_type, type_label, times, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			stallID, nullableID(o.AffiliateID), o.Title, o.Description, o.EventCarrd,
			o.StallType, o.TypeLabel, encodeEventTimes(o.Times), i)
		if err != nil {
			return err
		}
		newID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		keep[newID] = true
	}
	return deleteMissingOccupants(tx, stallID, keep)
}

// deleteMissingOccupants removes the pitch's occupants that the save left out,
// clearing any rally stamp that named one first (the link is a plain column, not
// an enforced FK - see migrateFestivalStallOccupants).
func deleteMissingOccupants(tx *sql.Tx, stallID int64, keep map[int64]bool) error {
	rows, err := tx.Query(`SELECT id FROM festival_stall_occupants WHERE stall_id = ?`, stallID)
	if err != nil {
		return err
	}
	var toDelete []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		if !keep[id] {
			toDelete = append(toDelete, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range toDelete {
		if _, err := tx.Exec(`UPDATE stamp_rally_stamps SET occupant_id = NULL WHERE occupant_id = ?`, id); err != nil {
			return err
		}
		// A raffle assigned to this occupant keeps its map - it is still part of the
		// festival - but loses the pitch, which no longer exists.
		if _, err := tx.Exec(`UPDATE raffles SET occupant_id = NULL WHERE occupant_id = ?`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM festival_stall_occupants WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

// DeleteFestivalMap removes a map and (via ON DELETE CASCADE) its stalls. It first
// clears the links held by any stamp rally that referenced the map or its stalls -
// both are plain columns, not enforced FKs (see migrateFestivalMaps) - so a rally
// survives the deletion of the map it was authored against, with its stamps
// falling back to their affiliates. Returns true if a row was deleted.
func (s *Store) DeleteFestivalMap(id int64) (bool, error) {
	tx, err := s.beginImmediate()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`UPDATE stamp_rally_stamps SET occupant_id = NULL WHERE occupant_id IN
		(SELECT o.id FROM festival_stall_occupants o
			JOIN festival_stalls st ON st.id = o.stall_id WHERE st.map_id = ?)`, id); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`UPDATE raffles SET festival_map_id = NULL, occupant_id = NULL
		WHERE festival_map_id = ?`, id); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`UPDATE stamp_rallies SET festival_map_id = NULL WHERE festival_map_id = ?`, id); err != nil {
		return false, err
	}
	res, err := tx.Exec(`DELETE FROM festival_maps WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return n > 0, nil
}

// SetFestivalMapStatus updates a map's publish status ("in_progress", "published"
// or "closed" - see the model.MapStatus* constants).
func (s *Store) SetFestivalMapStatus(id int64, status string) (bool, error) {
	res, err := s.db.Exec(`UPDATE festival_maps SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ListPublishedFestivalMaps returns the published maps (newest first) with their
// stall counts - the public list page. In-progress and closed maps never appear.
func (s *Store) ListPublishedFestivalMaps() ([]model.FestivalMap, error) {
	rows, err := s.db.Query(`SELECT m.id, m.title, m.slug, m.description, m.times, m.map_image, m.created_at,
			COALESCE((SELECT COUNT(*) FROM festival_stalls st WHERE st.map_id = m.id), 0)
		FROM festival_maps m WHERE m.status = ?
		ORDER BY m.created_at DESC, m.id DESC`, model.MapStatusPublished)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	maps := make([]model.FestivalMap, 0)
	for rows.Next() {
		var m model.FestivalMap
		var times string
		if err := rows.Scan(&m.ID, &m.Title, &m.Slug, &m.Description, &times, &m.MapImage,
			&m.CreatedAt, &m.StallCount); err != nil {
			return nil, err
		}
		m.Times = decodeEventTimes(times)
		m.Status = model.MapStatusPublished
		maps = append(maps, m)
	}
	return maps, rows.Err()
}

// -- Stamp Rally linkage ------------------------------------------------------

// MapStallStamp is one stamp a rally places on a festival map: which pitch
// OCCUPANT it belongs to and the art shown for it. Returned by
// ListRallyStampsForMap so the public map can badge the occupants that are part
// of a rally - the badge follows the occupant, since a pitch that changes hands
// between days hosts two different stalls.
type MapStallStamp struct {
	OccupantID int64
	Image      string
}

// StallRaffle is a raffle assigned to one of a map's pitch occupants. Status and
// the availability window come back raw: whether the raffle is RUNNING is a
// question about the clock, which this layer deliberately doesn't answer (see
// server/festivalmaps.go).
type StallRaffle struct {
	OccupantID    int64
	ID            int64
	Title         string
	PrizeImage    string
	Status        string
	AvailableFrom string
	AvailableTo   string
}

// ListRafflesForMap returns the raffles assigned to a map's occupants, keyed by
// occupant id. A pitch with more than one raffle keeps the newest, since the map
// panel links to a single one.
func (s *Store) ListRafflesForMap(mapID int64) (map[int64]StallRaffle, error) {
	rows, err := s.db.Query(`SELECT r.occupant_id, r.id, r.title, r.prize_image, r.status,
			r.available_from, r.available_to
		FROM raffles r
		JOIN festival_stall_occupants o ON o.id = r.occupant_id
		JOIN festival_stalls st ON st.id = o.stall_id
		WHERE st.map_id = ? AND r.occupant_id IS NOT NULL
		ORDER BY r.created_at ASC, r.id ASC`, mapID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byOccupant := make(map[int64]StallRaffle)
	for rows.Next() {
		var sr StallRaffle
		if err := rows.Scan(&sr.OccupantID, &sr.ID, &sr.Title, &sr.PrizeImage, &sr.Status,
			&sr.AvailableFrom, &sr.AvailableTo); err != nil {
			return nil, err
		}
		byOccupant[sr.OccupantID] = sr
	}
	return byOccupant, rows.Err()
}

// RallyForMap is the open stamp rally linked to a festival map, if any.
type RallyForMap struct {
	ID           int64
	Title        string
	PublicSignup bool
	// The availability window, so the caller can decide against the clock whether
	// the rally is actually RUNNING - the same way ListRafflesForMap reports a
	// raffle's window rather than pre-filtering on status alone.
	AvailableFrom string
	AvailableTo   string
}

// GetOpenRallyForMap returns the open stamp rally linked to a map, or nil when the
// map has none. A closed rally is deliberately excluded: the public map badges
// stalls only while their rally can still be stamped.
func (s *Store) GetOpenRallyForMap(mapID int64) (*RallyForMap, error) {
	var r RallyForMap
	var publicSignup int
	err := s.db.QueryRow(`SELECT id, title, public_signup, available_from, available_to
		FROM stamp_rallies
		WHERE festival_map_id = ? AND status != 'closed' ORDER BY id DESC LIMIT 1`, mapID).
		Scan(&r.ID, &r.Title, &publicSignup, &r.AvailableFrom, &r.AvailableTo)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.PublicSignup = publicSignup == 1
	return &r, nil
}

// ListRallyStampsForMap returns the stamps a rally has placed on festival-map
// occupants, keyed by occupant id. Stamps that name no occupant are skipped.
func (s *Store) ListRallyStampsForMap(rallyID int64) (map[int64]MapStallStamp, error) {
	rows, err := s.db.Query(`SELECT occupant_id, image FROM stamp_rally_stamps
		WHERE rally_id = ? AND occupant_id IS NOT NULL`, rallyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byOccupant := make(map[int64]MapStallStamp)
	for rows.Next() {
		var st MapStallStamp
		if err := rows.Scan(&st.OccupantID, &st.Image); err != nil {
			return nil, err
		}
		byOccupant[st.OccupantID] = st
	}
	return byOccupant, rows.Err()
}
