package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"app-suite/internal/model"
)

// -- Ema Plaques --------------------------------------------------------------
//
// A board (ema_boards) belongs to one festival map and takes plaques
// (ema_plaques) in windows (ema_windows). The one-plaque-per-person-per-window
// rule is a partial UNIQUE index rather than a read-then-insert check, so two
// submissions racing each other can't both land. A rejected plaque falls out of
// the index, which is what lets its author try again in the same window.

// ErrEmaPlaqueTaken is returned when the person already holds a plaque in the
// window being written to.
var ErrEmaPlaqueTaken = errors.New("a plaque is already recorded for this person in this window")

// ErrEmaWindowNotOnBoard is returned by UpdateEmaBoard when a save carries a window
// id that belongs to another board - a stale client or a spoofed id.
var ErrEmaWindowNotOnBoard = errors.New("window does not belong to this board")

// migrateEma (schema v70) adds the Ema plaque tables.
//
// ema_boards.festival_map_id cascades: a board exists only for its festival, and
// deleting the festival is deleting its board. Plaques keep a snapshot of their
// window's label, so editing or removing a window doesn't rewrite what a visitor
// was told their plaque was for.
func migrateEma(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS ema_boards (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			festival_map_id INTEGER NOT NULL UNIQUE REFERENCES festival_maps(id) ON DELETE CASCADE,
			title TEXT NOT NULL DEFAULT '',
			template_image TEXT NOT NULL DEFAULT '',
			instructions TEXT NOT NULL DEFAULT '',
			discord_link TEXT NOT NULL DEFAULT '',
			allow_self INTEGER NOT NULL DEFAULT 1,
			allow_site INTEGER NOT NULL DEFAULT 1,
			allow_artist INTEGER NOT NULL DEFAULT 1,
			artist_name TEXT NOT NULL DEFAULT 'Tara',
			status TEXT NOT NULL DEFAULT 'open',
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		)`,
		`CREATE TABLE IF NOT EXISTS ema_windows (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			board_id INTEGER NOT NULL REFERENCES ema_boards(id) ON DELETE CASCADE,
			label TEXT NOT NULL DEFAULT '',
			start_at TEXT NOT NULL,
			end_at TEXT NOT NULL,
			webhook_url TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ema_windows_board ON ema_windows(board_id)`,
		`CREATE TABLE IF NOT EXISTS ema_plaques (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			board_id INTEGER NOT NULL REFERENCES ema_boards(id) ON DELETE CASCADE,
			window_id INTEGER REFERENCES ema_windows(id) ON DELETE SET NULL,
			window_label TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL,
			character_name TEXT NOT NULL,
			world TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			image TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			reviewed_by TEXT NOT NULL DEFAULT '',
			reviewed_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ema_plaques_board ON ema_plaques(board_id, status)`,
		// One plaque per person per window, whichever way it was made. A rejected
		// plaque doesn't count, so its author can try again.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_ema_plaques_one_per_window
			ON ema_plaques(window_id, character_name COLLATE NOCASE, world COLLATE NOCASE)
			WHERE window_id IS NOT NULL AND status != 'rejected'`,
	}
	for _, q := range stmts {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

// -- Boards -------------------------------------------------------------------

const emaBoardColumns = `b.id, b.festival_map_id, COALESCE(m.title, ''), b.title, b.template_image,
	b.instructions, b.discord_link, b.allow_self, b.allow_site, b.allow_artist, b.artist_name,
	b.status, b.created_at`

const emaBoardFrom = `FROM ema_boards b LEFT JOIN festival_maps m ON m.id = b.festival_map_id`

func scanEmaBoard(sc rowScanner) (model.EmaBoard, error) {
	var b model.EmaBoard
	var self, site, artist int
	err := sc.Scan(&b.ID, &b.FestivalMapID, &b.FestivalMapName, &b.Title, &b.TemplateImage,
		&b.Instructions, &b.DiscordLink, &self, &site, &artist, &b.ArtistName, &b.Status, &b.CreatedAt)
	b.AllowSelf, b.AllowSite, b.AllowArtist = self == 1, site == 1, artist == 1
	return b, err
}

// ListEmaBoards returns every board, newest first, with the counts the admin list
// shows: site plaques waiting for review, and artist requests not yet done.
func (s *Store) ListEmaBoards(ctx context.Context) ([]model.EmaBoard, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+emaBoardColumns+`,
			(SELECT COUNT(*) FROM ema_plaques p WHERE p.board_id = b.id AND p.status = 'pending'),
			(SELECT COUNT(*) FROM ema_plaques p WHERE p.board_id = b.id AND p.status = 'requested')
		`+emaBoardFrom+` ORDER BY b.created_at DESC, b.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	boards := make([]model.EmaBoard, 0)
	for rows.Next() {
		var b model.EmaBoard
		var self, site, artist int
		if err := rows.Scan(&b.ID, &b.FestivalMapID, &b.FestivalMapName, &b.Title, &b.TemplateImage,
			&b.Instructions, &b.DiscordLink, &self, &site, &artist, &b.ArtistName, &b.Status, &b.CreatedAt,
			&b.PendingCount, &b.RequestedCount); err != nil {
			return nil, err
		}
		b.AllowSelf, b.AllowSite, b.AllowArtist = self == 1, site == 1, artist == 1
		boards = append(boards, b)
	}
	return boards, rows.Err()
}

// GetEmaBoard returns one board with its windows (nil if not found).
func (s *Store) GetEmaBoard(ctx context.Context, id int64) (*model.EmaBoard, error) {
	return s.getEmaBoardWhere(ctx, `b.id = ?`, id)
}

// GetEmaBoardForMap returns the board filed under a festival map (nil if none).
func (s *Store) GetEmaBoardForMap(ctx context.Context, mapID int64) (*model.EmaBoard, error) {
	return s.getEmaBoardWhere(ctx, `b.festival_map_id = ?`, mapID)
}

func (s *Store) getEmaBoardWhere(ctx context.Context, where string, arg any) (*model.EmaBoard, error) {
	b, err := scanEmaBoard(s.db.QueryRowContext(ctx, `SELECT `+emaBoardColumns+` `+emaBoardFrom+` WHERE `+where, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if b.Windows, err = s.listEmaWindows(ctx, b.ID); err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Store) listEmaWindows(ctx context.Context, boardID int64) ([]model.EmaWindow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, board_id, label, start_at, end_at, webhook_url, sort_order
		FROM ema_windows WHERE board_id = ? ORDER BY sort_order, start_at, id`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	windows := make([]model.EmaWindow, 0)
	for rows.Next() {
		var w model.EmaWindow
		if err := rows.Scan(&w.ID, &w.BoardID, &w.Label, &w.StartAt, &w.EndAt, &w.WebhookURL, &w.SortOrder); err != nil {
			return nil, err
		}
		windows = append(windows, w)
	}
	return windows, rows.Err()
}

// CreateEmaBoard inserts a board and its windows in one transaction.
func (s *Store) CreateEmaBoard(ctx context.Context, b *model.EmaBoard) (int64, error) {
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `INSERT INTO ema_boards (festival_map_id, title, template_image,
			instructions, discord_link, allow_self, allow_site, allow_artist, artist_name, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.FestivalMapID, b.Title, b.TemplateImage, b.Instructions, b.DiscordLink,
		boolToInt(b.AllowSelf), boolToInt(b.AllowSite), boolToInt(b.AllowArtist), b.ArtistName, b.Status)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i := range b.Windows {
		w := b.Windows[i]
		if _, err := tx.ExecContext(ctx, `INSERT INTO ema_windows (board_id, label, start_at, end_at, webhook_url, sort_order)
			VALUES (?, ?, ?, ?, ?, ?)`, id, w.Label, w.StartAt, w.EndAt, w.WebhookURL, i); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

// UpdateEmaBoard saves a board's editable fields and reconciles its windows by id
// in one transaction: an existing id is updated, id 0 is inserted, and a window
// left out is deleted (its plaques keep their label snapshot; window_id goes NULL).
// Keeping ids is what keeps a plaque attached to its window across an edit.
func (s *Store) UpdateEmaBoard(ctx context.Context, b *model.EmaBoard) error {
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `UPDATE ema_boards SET festival_map_id = ?, title = ?, template_image = ?,
			instructions = ?, discord_link = ?, allow_self = ?, allow_site = ?, allow_artist = ?,
			artist_name = ? WHERE id = ?`,
		b.FestivalMapID, b.Title, b.TemplateImage, b.Instructions, b.DiscordLink,
		boolToInt(b.AllowSelf), boolToInt(b.AllowSite), boolToInt(b.AllowArtist), b.ArtistName, b.ID); err != nil {
		return err
	}

	existing := make(map[int64]bool)
	rows, err := tx.QueryContext(ctx, `SELECT id FROM ema_windows WHERE board_id = ?`, b.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		existing[id] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	kept := make(map[int64]bool)
	for i := range b.Windows {
		w := b.Windows[i]
		if w.ID != 0 {
			if !existing[w.ID] {
				return ErrEmaWindowNotOnBoard
			}
			kept[w.ID] = true
			if _, err := tx.ExecContext(ctx, `UPDATE ema_windows SET label = ?, start_at = ?, end_at = ?,
					webhook_url = ?, sort_order = ? WHERE id = ?`,
				w.Label, w.StartAt, w.EndAt, w.WebhookURL, i, w.ID); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO ema_windows (board_id, label, start_at, end_at, webhook_url, sort_order)
			VALUES (?, ?, ?, ?, ?, ?)`, b.ID, w.Label, w.StartAt, w.EndAt, w.WebhookURL, i); err != nil {
			return err
		}
	}
	for id := range existing {
		if !kept[id] {
			if _, err := tx.ExecContext(ctx, `DELETE FROM ema_windows WHERE id = ?`, id); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// SetEmaBoardStatus opens or closes a board.
func (s *Store) SetEmaBoardStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE ema_boards SET status = ? WHERE id = ?`, status, id)
	return err
}

// DeleteEmaBoard deletes a board, its windows and its plaques, returning the image
// files the plaques still held so the caller can remove them from disk.
func (s *Store) DeleteEmaBoard(ctx context.Context, id int64) (deleted bool, images []string, err error) {
	tx, err := s.beginImmediate(ctx)
	if err != nil {
		return false, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `SELECT image FROM ema_plaques WHERE board_id = ? AND image != ''`, id)
	if err != nil {
		return false, nil, err
	}
	for rows.Next() {
		var img string
		if err := rows.Scan(&img); err != nil {
			_ = rows.Close()
			return false, nil, err
		}
		images = append(images, img)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, nil, err
	}
	_ = rows.Close()

	res, err := tx.ExecContext(ctx, `DELETE FROM ema_boards WHERE id = ?`, id)
	if err != nil {
		return false, nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, nil, err
	}
	return n > 0, images, tx.Commit()
}

// EmaImagesForMap returns the plaque images a festival's board still holds, read
// before the festival is deleted (the board cascades with it) so the files can be
// removed too.
func (s *Store) EmaImagesForMap(ctx context.Context, mapID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.image FROM ema_plaques p
		JOIN ema_boards b ON b.id = p.board_id
		WHERE b.festival_map_id = ? AND p.image != ''`, mapID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	images := make([]string, 0)
	for rows.Next() {
		var img string
		if err := rows.Scan(&img); err != nil {
			return nil, err
		}
		images = append(images, img)
	}
	return images, rows.Err()
}

// -- Plaques ------------------------------------------------------------------

const emaPlaqueColumns = `p.id, p.board_id, p.window_id, p.window_label, p.kind, p.character_name,
	p.world, p.description, p.image, p.status, p.reviewed_by, p.reviewed_at, p.created_at,
	COALESCE(m.title, '')`

const emaPlaqueFrom = `FROM ema_plaques p
	JOIN ema_boards b ON b.id = p.board_id
	LEFT JOIN festival_maps m ON m.id = b.festival_map_id`

func scanEmaPlaque(sc rowScanner) (model.EmaPlaque, error) {
	var p model.EmaPlaque
	var windowID sql.NullInt64
	err := sc.Scan(&p.ID, &p.BoardID, &windowID, &p.WindowLabel, &p.Kind, &p.CharacterName,
		&p.World, &p.Description, &p.Image, &p.Status, &p.ReviewedBy, &p.ReviewedAt, &p.CreatedAt,
		&p.FestivalMapName)
	if windowID.Valid {
		id := windowID.Int64
		p.WindowID = &id
	}
	return p, err
}

func (s *Store) listEmaPlaques(ctx context.Context, where string, args ...any) ([]model.EmaPlaque, error) {
	//nolint:gosec // `where` is one of this file's fixed predicates; every value is a ? placeholder
	rows, err := s.db.QueryContext(ctx, `SELECT `+emaPlaqueColumns+` `+emaPlaqueFrom+` WHERE `+where+`
		ORDER BY p.created_at DESC, p.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	plaques := make([]model.EmaPlaque, 0)
	for rows.Next() {
		p, err := scanEmaPlaque(rows)
		if err != nil {
			return nil, err
		}
		plaques = append(plaques, p)
	}
	return plaques, rows.Err()
}

// ListEmaPlaques returns a board's plaques, newest first.
func (s *Store) ListEmaPlaques(ctx context.Context, boardID int64) ([]model.EmaPlaque, error) {
	return s.listEmaPlaques(ctx, `p.board_id = ?`, boardID)
}

// ListEmaArtistRequests returns the artist's queue across every board, newest first.
func (s *Store) ListEmaArtistRequests(ctx context.Context) ([]model.EmaPlaque, error) {
	return s.listEmaPlaques(ctx, `p.kind = ?`, model.EmaKindArtist)
}

// ListEmaPlaquesFor returns one visitor's plaques on a board, matched on the whole
// name and world, ignoring case - the same identity the one-per-window rule uses.
func (s *Store) ListEmaPlaquesFor(ctx context.Context, boardID int64, name, world string) ([]model.EmaPlaque, error) {
	return s.listEmaPlaques(ctx, `p.board_id = ? AND p.character_name = ? COLLATE NOCASE AND p.world = ? COLLATE NOCASE`,
		boardID, name, world)
}

// GetEmaPlaque returns one plaque (nil if not found).
func (s *Store) GetEmaPlaque(ctx context.Context, id int64) (*model.EmaPlaque, error) {
	p, err := scanEmaPlaque(s.db.QueryRowContext(ctx, `SELECT `+emaPlaqueColumns+` `+emaPlaqueFrom+` WHERE p.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CreateEmaPlaque records a plaque, refusing a second one for the same person in
// the same window with ErrEmaPlaqueTaken.
func (s *Store) CreateEmaPlaque(ctx context.Context, p *model.EmaPlaque) (*model.EmaPlaque, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO ema_plaques (board_id, window_id, window_label, kind,
			character_name, world, description, image, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.BoardID, p.WindowID, p.WindowLabel, p.Kind, p.CharacterName, p.World, p.Description, p.Image, p.Status)
	if isUniqueViolation(err) {
		return nil, ErrEmaPlaqueTaken
	}
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetEmaPlaque(ctx, id)
}

// SetEmaPlaqueStatus moves a plaque from one status to another, only while it is
// still in `from` - the claim an approval takes before posting, and its release
// when the post fails. Reports whether it applied.
func (s *Store) SetEmaPlaqueStatus(ctx context.Context, id int64, from, to string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE ema_plaques SET status = ? WHERE id = ? AND status = ?`, to, id, from)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// FinishEmaPlaque moves a plaque to its final status, records who did it, and
// clears its image (the file is deleted by the caller - Discord holds the posted
// copy, and a rejected one is never used). Conditional on `from`, like
// SetEmaPlaqueStatus. Reports whether it applied.
func (s *Store) FinishEmaPlaque(ctx context.Context, id int64, from, to, by string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE ema_plaques SET status = ?, image = '', reviewed_by = ?, reviewed_at = ?
		WHERE id = ? AND status = ?`, to, by, time.Now().UTC().Format(time.RFC3339), id, from)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// SetEmaRequestComplete marks an artist request complete (or reopens it).
func (s *Store) SetEmaRequestComplete(ctx context.Context, id int64, complete bool, by string) (bool, error) {
	from, to, at := model.EmaStatusRequested, model.EmaStatusComplete, time.Now().UTC().Format(time.RFC3339)
	if !complete {
		from, to, by, at = model.EmaStatusComplete, model.EmaStatusRequested, "", ""
	}
	res, err := s.db.ExecContext(ctx, `UPDATE ema_plaques SET status = ?, reviewed_by = ?, reviewed_at = ?
		WHERE id = ? AND kind = ? AND status = ?`, to, by, at, id, model.EmaKindArtist, from)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
