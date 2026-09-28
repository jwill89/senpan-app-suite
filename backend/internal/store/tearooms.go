package store

import (
	"database/sql"
	"errors"

	"app-suite/internal/model"
)

// -- Tea Rooms ----------------------------------------------------------------
//
// A tea room is a single-table entity (see model.TeaRoom): a bookable room with a
// per-half-hour gil cost, hashtags, a markdown description, status flags, an image,
// and a Discord embed colour. Admins keep a drag-orderable list; the shared Discord
// webhook they post to lives in the settings table, not here.

// teaRoomColumns is the shared SELECT column list for the tea_rooms table.
const teaRoomColumns = `id, name, subtitle, room_owner, room_number, cost_per_half_hour, hashtags, description,
	seasonal, open, lockable, discounted, locked, locked_until, image, color, sort_order, created_at`

// scanTeaRoom scans one row (in teaRoomColumns order), decoding the 0/1 flag
// columns into bools.
func scanTeaRoom(sc rowScanner) (*model.TeaRoom, error) {
	var t model.TeaRoom
	var seasonal, open, lockable, discounted, locked int
	if err := sc.Scan(&t.ID, &t.Name, &t.Subtitle, &t.RoomOwner, &t.RoomNumber, &t.CostPerHalfHour, &t.Hashtags, &t.Description,
		&seasonal, &open, &lockable, &discounted, &locked, &t.LockedUntil, &t.Image, &t.Color, &t.SortOrder, &t.CreatedAt); err != nil {
		return nil, err
	}
	t.Seasonal = seasonal != 0
	t.Open = open != 0
	t.Lockable = lockable != 0
	t.Discounted = discounted != 0
	t.Locked = locked != 0
	return &t, nil
}

// ListTeaRooms returns every tea room in the admin's chosen order: sort_order
// ascending, then newest-first as a tie-break (so a freshly created room appears
// at the top until the admin drags to reorder). Mirrors ListAnnouncements.
func (s *Store) ListTeaRooms() ([]model.TeaRoom, error) {
	rows, err := s.db.Query(
		`SELECT ` + teaRoomColumns + ` FROM tea_rooms ORDER BY sort_order ASC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.TeaRoom, 0)
	for rows.Next() {
		t, err := scanTeaRoom(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// GetTeaRoom returns a single tea room by id, or nil if it doesn't exist.
func (s *Store) GetTeaRoom(id int64) (*model.TeaRoom, error) {
	row := s.db.QueryRow(`SELECT `+teaRoomColumns+` FROM tea_rooms WHERE id = ?`, id)
	t, err := scanTeaRoom(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// GetTeaRoomByNumber returns the room with the given room_number, or nil if none.
// room_number is unique, so at most one matches; the ORDER BY is a defensive
// tie-break. It backs the public single-room lookup + the uniqueness check.
func (s *Store) GetTeaRoomByNumber(number string) (*model.TeaRoom, error) {
	row := s.db.QueryRow(`SELECT `+teaRoomColumns+`
		FROM tea_rooms WHERE room_number = ? ORDER BY sort_order ASC, id ASC LIMIT 1`, number)
	t, err := scanTeaRoom(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// CreateTeaRoom inserts a new tea room and returns its ID.
func (s *Store) CreateTeaRoom(t *model.TeaRoom) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO tea_rooms
			(name, subtitle, room_owner, room_number, cost_per_half_hour, hashtags, description,
			 seasonal, open, lockable, discounted, locked, locked_until, image, color)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Name, t.Subtitle, t.RoomOwner, t.RoomNumber, t.CostPerHalfHour, t.Hashtags, t.Description,
		boolToInt(t.Seasonal), boolToInt(t.Open), boolToInt(t.Lockable), boolToInt(t.Discounted),
		boolToInt(t.Locked), t.LockedUntil, t.Image, t.Color)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateTeaRoom updates a tea room's editable fields (everything but id/sort_order/
// created_at). The open/discounted flags and the lock are editable here too,
// alongside the dedicated single-field setters (SetTeaRoomOpen/
// SetTeaRoomDiscounted/SetTeaRoomLock).
func (s *Store) UpdateTeaRoom(t *model.TeaRoom) error {
	_, err := s.db.Exec(
		`UPDATE tea_rooms SET name = ?, subtitle = ?, room_owner = ?, room_number = ?, cost_per_half_hour = ?, hashtags = ?,
			description = ?, seasonal = ?, open = ?, lockable = ?, discounted = ?, locked = ?, locked_until = ?,
			image = ?, color = ?
		 WHERE id = ?`,
		t.Name, t.Subtitle, t.RoomOwner, t.RoomNumber, t.CostPerHalfHour, t.Hashtags, t.Description,
		boolToInt(t.Seasonal), boolToInt(t.Open), boolToInt(t.Lockable), boolToInt(t.Discounted),
		boolToInt(t.Locked), t.LockedUntil, t.Image, t.Color, t.ID)
	return err
}

// SetTeaRoomOpen flips a room's open/closed flag without re-saving the whole room.
func (s *Store) SetTeaRoomOpen(id int64, open bool) error {
	_, err := s.db.Exec(`UPDATE tea_rooms SET open = ? WHERE id = ?`, boolToInt(open), id)
	return err
}

// SetTeaRoomDiscounted flips a room's discounted flag without re-saving the room.
func (s *Store) SetTeaRoomDiscounted(id int64, discounted bool) error {
	_, err := s.db.Exec(`UPDATE tea_rooms SET discounted = ? WHERE id = ?`, boolToInt(discounted), id)
	return err
}

// SetTeaRoomLock locks or unlocks a room without re-saving the whole room.
// lockedUntil is a UTC RFC-3339 instant, or "" for a lock with no expiry (it
// stands until someone unlocks the room). Unlocking always clears the expiry, so
// a stale time can't outlive the lock it belonged to.
func (s *Store) SetTeaRoomLock(id int64, locked bool, lockedUntil string) error {
	if !locked {
		lockedUntil = ""
	}
	_, err := s.db.Exec(`UPDATE tea_rooms SET locked = ?, locked_until = ? WHERE id = ?`,
		boolToInt(locked), lockedUntil, id)
	return err
}

// ExpiringTeaRoomLocks returns every room currently locked WITH an expiry set -
// the only rooms whose lock can lift on its own. The caller compares the times
// (see server.parseTeaRoomLockTime), so a value SQLite's date functions can't
// parse is still visible rather than silently never coming due.
func (s *Store) ExpiringTeaRoomLocks() ([]model.TeaRoom, error) {
	rows, err := s.db.Query(`SELECT ` + teaRoomColumns + `
		FROM tea_rooms WHERE locked = 1 AND locked_until != '' ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.TeaRoom, 0)
	for rows.Next() {
		t, err := scanTeaRoom(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// ExpireTeaRoomLock clears a lock that has run out, and reports whether it
// actually did. The update is conditional on the room still holding the SAME
// expiry the caller saw: an admin who relocks or re-times the room mid-sweep owns
// the lock from then on, and their change must not be undone by a decision made
// from a snapshot taken before it. Mirrors AdvanceAnnouncement's applied flag.
func (s *Store) ExpireTeaRoomLock(id int64, expectedUntil string) (bool, error) {
	res, err := s.db.Exec(
		`UPDATE tea_rooms SET locked = 0, locked_until = ''
		 WHERE id = ? AND locked = 1 AND locked_until = ?`, id, expectedUntil)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteTeaRoom removes a tea room by ID. Returns true if a row was deleted. The
// image file lives in a centrally-managed image category (System -> Images), so it
// is intentionally left intact for reuse.
func (s *Store) DeleteTeaRoom(id int64) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM tea_rooms WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// BulkReorderTeaRooms rewrites the sort_order of the given rooms so they list in
// the supplied id order (index 0 = top), in a single transaction. Unknown ids are
// no-ops. Mirrors BulkReorderAnnouncements.
func (s *Store) BulkReorderTeaRooms(ids []int64) error {
	tx, err := s.beginImmediate()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE tea_rooms SET sort_order = ? WHERE id = ?`, i, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
