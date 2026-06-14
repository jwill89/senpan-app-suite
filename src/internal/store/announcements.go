package store

import (
	"database/sql"
	"errors"
	"time"

	"app-suite/internal/model"
)

// ── Announcement types ──────────────────────────────────────────────────────
//
// An announcement type is a named Discord destination (label + channel webhook).
// Every announcement references one type and posts to its webhook.

// ListAnnouncementTypes returns all announcement types, newest first.
func (s *Store) ListAnnouncementTypes() ([]model.AnnouncementType, error) {
	rows, err := s.db.Query(
		`SELECT id, name, webhook_url, created_at FROM announcement_types ORDER BY id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	types := make([]model.AnnouncementType, 0)
	for rows.Next() {
		var t model.AnnouncementType
		if err := rows.Scan(&t.ID, &t.Name, &t.WebhookURL, &t.CreatedAt); err != nil {
			return nil, err
		}
		types = append(types, t)
	}
	return types, rows.Err()
}

// GetAnnouncementType returns a single type by ID, or nil if it does not exist.
func (s *Store) GetAnnouncementType(id int64) (*model.AnnouncementType, error) {
	var t model.AnnouncementType
	err := s.db.QueryRow(
		`SELECT id, name, webhook_url, created_at FROM announcement_types WHERE id = ?`, id,
	).Scan(&t.ID, &t.Name, &t.WebhookURL, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateAnnouncementType inserts a new type and returns its ID.
func (s *Store) CreateAnnouncementType(name, webhookURL string) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO announcement_types (name, webhook_url) VALUES (?, ?)`, name, webhookURL,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateAnnouncementType updates a type's name and webhook.
func (s *Store) UpdateAnnouncementType(id int64, name, webhookURL string) error {
	_, err := s.db.Exec(
		`UPDATE announcement_types SET name = ?, webhook_url = ? WHERE id = ?`, name, webhookURL, id,
	)
	return err
}

// DeleteAnnouncementType removes a type by ID. Returns true if a row was deleted.
func (s *Store) DeleteAnnouncementType(id int64) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM announcement_types WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CountAnnouncementsByType returns how many announcements reference a type — used
// to block deleting a type that's still in use.
func (s *Store) CountAnnouncementsByType(typeID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM announcements WHERE type_id = ?`, typeID).Scan(&n)
	return n, err
}

// ── Announcements ───────────────────────────────────────────────────────────

// announcementColumns is the shared SELECT column list for the announcements
// table (alias a), with the joined type name appended.
const announcementColumns = `a.id, a.type_id, a.title, a.details, a.image, a.color, a.start_local, a.end_local,
	a.start_at, a.end_at, a.schedule_kind, a.timezone, a.once_local, a.schedule_minutes,
	a.schedule_weekdays, a.schedule_week_of_month,
	a.next_post_at, a.skip_next, a.active, a.last_posted_at, a.created_at, COALESCE(t.name, '')`

// scanAnnouncement scans one row (in announcementColumns order) into a model.
func scanAnnouncement(sc interface{ Scan(...any) error }) (model.Announcement, error) {
	var a model.Announcement
	var skip, active int
	var lastPosted sql.NullString
	err := sc.Scan(&a.ID, &a.TypeID, &a.Title, &a.Details, &a.Image, &a.Color, &a.StartLocal, &a.EndLocal,
		&a.StartAt, &a.EndAt, &a.ScheduleKind, &a.Timezone, &a.OnceLocal, &a.ScheduleMinutes,
		&a.ScheduleWeekdays, &a.ScheduleWeekOfMonth,
		&a.NextPostAt, &skip, &active, &lastPosted, &a.CreatedAt, &a.TypeName)
	if err != nil {
		return a, err
	}
	a.SkipNext = skip != 0
	a.Active = active != 0
	if lastPosted.Valid {
		a.LastPostedAt = lastPosted.String
	}
	return a, nil
}

// ListAnnouncements returns all announcements (with their type name), newest first.
func (s *Store) ListAnnouncements() ([]model.Announcement, error) {
	rows, err := s.db.Query(
		`SELECT ` + announcementColumns + ` FROM announcements a
		   LEFT JOIN announcement_types t ON t.id = a.type_id
		   ORDER BY a.id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.Announcement, 0)
	for rows.Next() {
		a, err := scanAnnouncement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAnnouncement returns a single announcement by ID, or nil if it doesn't exist.
func (s *Store) GetAnnouncement(id int64) (*model.Announcement, error) {
	row := s.db.QueryRow(
		`SELECT `+announcementColumns+` FROM announcements a
		   LEFT JOIN announcement_types t ON t.id = a.type_id
		   WHERE a.id = ?`, id,
	)
	a, err := scanAnnouncement(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// CreateAnnouncement inserts a new announcement and returns its ID.
func (s *Store) CreateAnnouncement(a *model.Announcement) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO announcements
			(type_id, title, details, image, color, start_local, end_local, start_at, end_at,
			 schedule_kind, timezone, once_local, schedule_minutes, schedule_weekdays, schedule_week_of_month,
			 next_post_at, active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.TypeID, a.Title, a.Details, a.Image, a.Color, a.StartLocal, a.EndLocal, a.StartAt, a.EndAt,
		a.ScheduleKind, a.Timezone, a.OnceLocal, a.ScheduleMinutes, a.ScheduleWeekdays, a.ScheduleWeekOfMonth,
		a.NextPostAt, boolToInt(a.Active),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateAnnouncement updates an announcement's editable fields. It does not touch
// last_posted_at (that's stamped only when actually posting).
func (s *Store) UpdateAnnouncement(a *model.Announcement) error {
	_, err := s.db.Exec(
		`UPDATE announcements SET type_id = ?, title = ?, details = ?, image = ?, color = ?,
			start_local = ?, end_local = ?, start_at = ?, end_at = ?, schedule_kind = ?,
			timezone = ?, once_local = ?, schedule_minutes = ?, schedule_weekdays = ?,
			schedule_week_of_month = ?, next_post_at = ?, skip_next = ?, active = ?
		 WHERE id = ?`,
		a.TypeID, a.Title, a.Details, a.Image, a.Color, a.StartLocal, a.EndLocal, a.StartAt, a.EndAt,
		a.ScheduleKind, a.Timezone, a.OnceLocal, a.ScheduleMinutes, a.ScheduleWeekdays,
		a.ScheduleWeekOfMonth, a.NextPostAt, boolToInt(a.SkipNext), boolToInt(a.Active), a.ID,
	)
	return err
}

// DeleteAnnouncement removes an announcement by ID. Returns true if a row was deleted.
func (s *Store) DeleteAnnouncement(id int64) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM announcements WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DueAnnouncements returns active announcements whose next_post_at has arrived
// (set and at/before now), soonest first — the scheduler's work queue.
func (s *Store) DueAnnouncements(now time.Time) ([]model.Announcement, error) {
	rows, err := s.db.Query(
		`SELECT `+announcementColumns+` FROM announcements a
		   LEFT JOIN announcement_types t ON t.id = a.type_id
		   WHERE a.active = 1 AND a.next_post_at != '' AND datetime(a.next_post_at) <= datetime(?)
		   ORDER BY a.next_post_at ASC, a.id ASC`,
		now.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.Announcement, 0)
	for rows.Next() {
		a, err := scanAnnouncement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// MarkAnnouncementPosted stamps last_posted_at and updates the schedule cursor:
// the next instant to fire (empty for a finished one-time) and whether it stays
// active.
func (s *Store) MarkAnnouncementPosted(id int64, nextPostAt string, active bool) error {
	_, err := s.db.Exec(
		`UPDATE announcements SET last_posted_at = CURRENT_TIMESTAMP, next_post_at = ?, active = ?
		 WHERE id = ?`, nextPostAt, boolToInt(active), id,
	)
	return err
}

// AdvanceAnnouncement updates only the schedule cursor (next instant + active +
// skip flag) without stamping a post — used when a scheduled occurrence is skipped.
func (s *Store) AdvanceAnnouncement(id int64, nextPostAt string, active, skipNext bool) error {
	_, err := s.db.Exec(
		`UPDATE announcements SET next_post_at = ?, active = ?, skip_next = ? WHERE id = ?`,
		nextPostAt, boolToInt(active), boolToInt(skipNext), id,
	)
	return err
}

// SetAnnouncementSkip flags (or clears) the "skip the next occurrence" marker.
func (s *Store) SetAnnouncementSkip(id int64, skip bool) error {
	_, err := s.db.Exec(`UPDATE announcements SET skip_next = ? WHERE id = ?`, boolToInt(skip), id)
	return err
}

// TouchAnnouncementPosted stamps last_posted_at without touching the schedule —
// used by a manual "send now" that should not advance the recurrence cursor.
func (s *Store) TouchAnnouncementPosted(id int64) error {
	_, err := s.db.Exec(
		`UPDATE announcements SET last_posted_at = CURRENT_TIMESTAMP WHERE id = ?`, id,
	)
	return err
}

// boolToInt maps a bool to SQLite's 0/1 integer representation.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
