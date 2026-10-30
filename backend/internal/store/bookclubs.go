package store

import (
	"context"
	"database/sql"
	"errors"

	"app-suite/internal/model"
)

// -- Reading lists -----------------------------------------------------------

// ListReadingLists returns all reading lists for a book club, newest first.
// Items are not loaded (use GetReadingList for the full detail).
func (s *Store) ListReadingLists(ctx context.Context, clubSlug string) ([]model.ReadingList, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, club_slug, title, created_at FROM reading_lists WHERE club_slug = ? ORDER BY created_at DESC, id DESC`,
		clubSlug,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lists := make([]model.ReadingList, 0)
	for rows.Next() {
		var l model.ReadingList
		if err := rows.Scan(&l.ID, &l.ClubSlug, &l.Title, &l.CreatedAt); err != nil {
			return nil, err
		}
		lists = append(lists, l)
	}
	return lists, rows.Err()
}

// GetReadingList retrieves a single reading list with its items (ordered by
// sort_order then id). Returns nil if the list does not exist.
func (s *Store) GetReadingList(ctx context.Context, id int64) (*model.ReadingList, error) {
	var l model.ReadingList
	err := s.db.QueryRowContext(ctx,
		`SELECT id, club_slug, title, created_at FROM reading_lists WHERE id = ?`, id,
	).Scan(&l.ID, &l.ClubSlug, &l.Title, &l.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	items, err := s.listReadingListItems(ctx, id)
	if err != nil {
		return nil, err
	}
	l.Items = items
	return &l, nil
}

// listReadingListItems loads the items for a reading list, decoding the JSON
// sources column into each item's Sources slice.
func (s *Store) listReadingListItems(ctx context.Context, listID int64) ([]model.ReadingListItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, list_id, cover_image, title, summary, format, genres, tropes, chapters, comments, sources, sort_order
		   FROM reading_list_items WHERE list_id = ? ORDER BY sort_order ASC, id ASC`,
		listID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.ReadingListItem, 0)
	for rows.Next() {
		var it model.ReadingListItem
		var sourcesJSON string
		if err := rows.Scan(&it.ID, &it.ListID, &it.CoverImage, &it.Title, &it.Summary,
			&it.Format, &it.Genres, &it.Tropes, &it.Chapters, &it.Comments, &sourcesJSON, &it.SortOrder); err != nil {
			return nil, err
		}
		it.Sources = decodeSources(sourcesJSON)
		items = append(items, it)
	}
	return items, rows.Err()
}

// CreateReadingList inserts a new reading list and returns its ID.
func (s *Store) CreateReadingList(ctx context.Context, clubSlug, title string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO reading_lists (club_slug, title) VALUES (?, ?)`, clubSlug, title,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateReadingListTitle renames an existing reading list.
func (s *Store) UpdateReadingListTitle(ctx context.Context, id int64, title string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE reading_lists SET title = ? WHERE id = ?`, title, id)
	return err
}

// DeleteReadingList removes a reading list and its items (cascade). Returns
// true if a row was deleted.
func (s *Store) DeleteReadingList(ctx context.Context, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM reading_lists WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// -- Reading list items ------------------------------------------------------

// CreateReadingListItem inserts a new item at the end of its list (next
// sort_order) and returns its ID. Sources are stored as JSON.
func (s *Store) CreateReadingListItem(ctx context.Context, it *model.ReadingListItem) (int64, error) {
	var nextOrder int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order), 0) + 1 FROM reading_list_items WHERE list_id = ?`, it.ListID,
	).Scan(&nextOrder); err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO reading_list_items (list_id, cover_image, title, summary, format, genres, tropes, chapters, comments, sources, sort_order)
		   VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.ListID, it.CoverImage, it.Title, it.Summary, it.Format, it.Genres,
		it.Tropes, it.Chapters, it.Comments, encodeSources(it.Sources), nextOrder,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateReadingListItem updates an item's editable fields (not its list or order),
// scoped to the list it.ListID names. Returns true if a row matched.
//
// The list scope is what stops an item id from reaching across lists - and so
// across book clubs, since a list belongs to exactly one club and the clubs are
// separately permissioned. The route validates that the LIST belongs to the club
// in the path; without the scope here, a user holding one club's permission could
// pass any item id under a list they legitimately own and edit another club's item.
func (s *Store) UpdateReadingListItem(ctx context.Context, it *model.ReadingListItem) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reading_list_items SET cover_image = ?, title = ?, summary = ?, format = ?, genres = ?, tropes = ?, chapters = ?, comments = ?, sources = ? WHERE id = ? AND list_id = ?`,
		it.CoverImage, it.Title, it.Summary, it.Format, it.Genres,
		it.Tropes, it.Chapters, it.Comments, encodeSources(it.Sources), it.ID, it.ListID,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteReadingListItem removes a single item from the given list. Returns true if
// a row was deleted. Scoped to listID for the same cross-club reason as
// UpdateReadingListItem.
func (s *Store) DeleteReadingListItem(ctx context.Context, listID, itemID int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM reading_list_items WHERE id = ? AND list_id = ?`, itemID, listID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CountReadingListItemsByCover returns how many items still reference a cover URL.
// Cover images are uploaded under their original filename (so two items can share a
// file), so callers check this after deleting an item before removing its cover
// file - a shared file must survive while another item still points at it.
func (s *Store) CountReadingListItemsByCover(ctx context.Context, coverImage string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM reading_list_items WHERE cover_image = ?`, coverImage).Scan(&n)
	return n, err
}

// These wrap the shared generic JSON-array codecs (jsonarray.go): always a valid
// array (never "null"), never nil.
func encodeSources(sources []model.ReadingListSource) string { return encodeJSONArray(sources) }
func decodeSources(raw string) []model.ReadingListSource {
	return decodeJSONArray[model.ReadingListSource](raw)
}
