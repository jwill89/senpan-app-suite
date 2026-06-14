package store

import (
	"database/sql"
	"errors"
	"time"

	"app-suite/internal/model"
)

// ── Book club events ────────────────────────────────────────────────────────
//
// Scheduled event posts for a book club. The absolute instants (start_at /
// post_at) are computed by the server layer from the admin's wall-clock input +
// IANA timezone and stored as UTC RFC-3339 strings before being persisted here.

// eventColumns is the shared SELECT column list for book_club_events.
const eventColumns = `id, club_slug, title, start_local, timezone, length_hours, location, details, image,
	post_at_local, start_at, post_at, posted, posted_at, created_at`

// scanEvent scans one book_club_events row (in eventColumns order) into a model.
func scanEvent(sc interface{ Scan(...any) error }) (model.BookClubEvent, error) {
	var ev model.BookClubEvent
	var posted int
	var postedAt sql.NullString
	err := sc.Scan(&ev.ID, &ev.ClubSlug, &ev.Title, &ev.StartLocal, &ev.Timezone,
		&ev.LengthHours, &ev.Location, &ev.Details, &ev.Image, &ev.PostAtLocal, &ev.StartAt,
		&ev.PostAt, &posted, &postedAt, &ev.CreatedAt)
	if err != nil {
		return ev, err
	}
	ev.Posted = posted != 0
	if postedAt.Valid {
		ev.PostedAt = postedAt.String
	}
	return ev, nil
}

// ListBookClubEvents returns all events for a club, soonest start first.
func (s *Store) ListBookClubEvents(clubSlug string) ([]model.BookClubEvent, error) {
	rows, err := s.db.Query(
		`SELECT `+eventColumns+` FROM book_club_events WHERE club_slug = ?
		   ORDER BY start_at ASC, id ASC`,
		clubSlug,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]model.BookClubEvent, 0)
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// GetBookClubEvent returns a single event by ID, or nil if it does not exist.
func (s *Store) GetBookClubEvent(id int64) (*model.BookClubEvent, error) {
	row := s.db.QueryRow(`SELECT `+eventColumns+` FROM book_club_events WHERE id = ?`, id)
	ev, err := scanEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ev, nil
}

// CreateBookClubEvent inserts a new event and returns its ID.
func (s *Store) CreateBookClubEvent(ev *model.BookClubEvent) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO book_club_events
			(club_slug, title, start_local, timezone, length_hours, location, details, image,
			 post_at_local, start_at, post_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.ClubSlug, ev.Title, ev.StartLocal, ev.Timezone, ev.LengthHours, ev.Location,
		ev.Details, ev.Image, ev.PostAtLocal, ev.StartAt, ev.PostAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateBookClubEvent updates an event's editable fields. It does not change the
// posted state (use MarkBookClubEventPosted for that) so re-scheduling an
// already-posted event won't silently re-post it.
func (s *Store) UpdateBookClubEvent(ev *model.BookClubEvent) error {
	_, err := s.db.Exec(
		`UPDATE book_club_events SET title = ?, start_local = ?, timezone = ?, length_hours = ?,
			location = ?, details = ?, image = ?, post_at_local = ?, start_at = ?, post_at = ?
		 WHERE id = ?`,
		ev.Title, ev.StartLocal, ev.Timezone, ev.LengthHours, ev.Location, ev.Details, ev.Image,
		ev.PostAtLocal, ev.StartAt, ev.PostAt, ev.ID,
	)
	return err
}

// DeleteBookClubEvent removes an event by ID. Returns true if a row was deleted.
func (s *Store) DeleteBookClubEvent(id int64) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM book_club_events WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DueBookClubEvents returns unposted events whose post time has arrived
// (post_at set and at/before now), soonest first — the scheduler's work queue.
// post_at is a UTC RFC-3339 string; datetime() normalizes it for the comparison.
func (s *Store) DueBookClubEvents(now time.Time) ([]model.BookClubEvent, error) {
	rows, err := s.db.Query(
		`SELECT `+eventColumns+` FROM book_club_events
		   WHERE posted = 0 AND post_at != '' AND datetime(post_at) <= datetime(?)
		   ORDER BY post_at ASC, id ASC`,
		now.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]model.BookClubEvent, 0)
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// MarkBookClubEventPosted flags an event as posted and stamps the posted time.
func (s *Store) MarkBookClubEventPosted(id int64) error {
	_, err := s.db.Exec(
		`UPDATE book_club_events SET posted = 1, posted_at = CURRENT_TIMESTAMP WHERE id = ?`, id,
	)
	return err
}
