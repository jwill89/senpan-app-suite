package store

import (
	"database/sql"
	"errors"

	"app-suite/internal/model"
)

// ── Affiliates ───────────────────────────────────────────────────────────────
//
// An affiliate is a partner establishment (see model.Affiliate). It's a single
// table: owners and opening hours are stored as JSON arrays in the owners/hours
// columns (mirroring reading_list_items.sources) rather than sub-tables, since
// they're small, always loaded with the row, and edited as a set.

// affiliateColumns is the shared SELECT column list (order matches scanAffiliate).
const affiliateColumns = `id, name, owners, location, timezone, hours, details, logo, screenshot,
	embed_color, discord_link, carrd_link, sort_order, created_at`

// ListAffiliates returns every affiliate in the admin's manual drag order, then
// alphabetically by name (so a fresh install / never-reordered list stays
// alphabetical until dragged), each with its owners and hours decoded.
func (s *Store) ListAffiliates() ([]model.Affiliate, error) {
	rows, err := s.db.Query(
		`SELECT ` + affiliateColumns + `
		   FROM affiliates ORDER BY sort_order ASC, name COLLATE NOCASE ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	affiliates := make([]model.Affiliate, 0)
	for rows.Next() {
		a, err := scanAffiliate(rows)
		if err != nil {
			return nil, err
		}
		affiliates = append(affiliates, *a)
	}
	return affiliates, rows.Err()
}

// GetAffiliate retrieves a single affiliate by id. Returns nil if not found.
func (s *Store) GetAffiliate(id int64) (*model.Affiliate, error) {
	row := s.db.QueryRow(
		`SELECT `+affiliateColumns+` FROM affiliates WHERE id = ?`, id)
	a, err := scanAffiliate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

// CreateAffiliate inserts a new affiliate and returns its ID. Owners and hours
// are stored as JSON arrays.
func (s *Store) CreateAffiliate(a *model.Affiliate) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO affiliates
		   (name, owners, location, timezone, hours, details, logo, screenshot, embed_color, discord_link, carrd_link)
		   VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Name, encodeStrings(a.Owners), a.Location, a.Timezone,
		encodeHours(a.Hours), a.Details, a.Logo, a.Screenshot,
		a.EmbedColor, a.DiscordLink, a.CarrdLink)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateAffiliate updates an affiliate's editable fields (everything but
// id/created_at/sort_order — sort_order is managed by BulkReorderAffiliates, so an
// edit must not reset the drag order).
func (s *Store) UpdateAffiliate(a *model.Affiliate) error {
	_, err := s.db.Exec(
		`UPDATE affiliates SET name = ?, owners = ?, location = ?, timezone = ?,
		   hours = ?, details = ?, logo = ?, screenshot = ?,
		   embed_color = ?, discord_link = ?, carrd_link = ? WHERE id = ?`,
		a.Name, encodeStrings(a.Owners), a.Location, a.Timezone,
		encodeHours(a.Hours), a.Details, a.Logo, a.Screenshot,
		a.EmbedColor, a.DiscordLink, a.CarrdLink, a.ID)
	return err
}

// BulkReorderAffiliates rewrites sort_order to match the given id order (index 0 =
// top), in a single transaction. Ids not present are left untouched.
func (s *Store) BulkReorderAffiliates(ids []int64) error {
	tx, err := s.beginImmediate()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE affiliates SET sort_order = ? WHERE id = ?`, i, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteAffiliate removes an affiliate. Returns true if a row was deleted. The
// logo/screenshot files live in centrally-managed image categories (System →
// Images), so they're intentionally left intact for reuse.
func (s *Store) DeleteAffiliate(id int64) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM affiliates WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// rowScanner is the shared interface of *sql.Row and *sql.Rows (just Scan), so a
// single affiliate scan helper serves both the single-row Get and the list Query.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanAffiliate scans one affiliate row, decoding the JSON owners/hours columns.
func scanAffiliate(sc rowScanner) (*model.Affiliate, error) {
	var a model.Affiliate
	var ownersJSON, hoursJSON string
	if err := sc.Scan(&a.ID, &a.Name, &ownersJSON, &a.Location, &a.Timezone,
		&hoursJSON, &a.Details, &a.Logo, &a.Screenshot,
		&a.EmbedColor, &a.DiscordLink, &a.CarrdLink, &a.SortOrder, &a.CreatedAt); err != nil {
		return nil, err
	}
	a.Owners = decodeStrings(ownersJSON)
	a.Hours = decodeHours(hoursJSON)
	return &a, nil
}

// These wrap the shared generic JSON-array codecs (jsonarray.go), which always
// produce a valid array (never "null") and never return nil.
func encodeStrings(in []string) string               { return encodeJSONArray(in) }
func decodeStrings(raw string) []string              { return decodeJSONArray[string](raw) }
func encodeHours(hours []model.AffiliateHour) string { return encodeJSONArray(hours) }
func decodeHours(raw string) []model.AffiliateHour   { return decodeJSONArray[model.AffiliateHour](raw) }
