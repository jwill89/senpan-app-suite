package store

import (
	"context"
	"database/sql"
	"errors"

	"app-suite/internal/model"
)

// -- Affiliates ---------------------------------------------------------------
//
// The affiliates' own API reads and writes through sqlc (internal/db); what stays
// here is the one lookup the festival map's public card still uses.

// affiliateColumns is the SELECT column list (order matches scanAffiliate).
const affiliateColumns = `id, name, subtitle, owners, location, timezone, hours, details, logo, screenshot,
	embed_color, discord_link, carrd_link, sort_order, created_at`

// GetAffiliate retrieves a single affiliate by id. Returns nil if not found.
func (s *Store) GetAffiliate(ctx context.Context, id int64) (*model.Affiliate, error) {
	row := s.read.QueryRowContext(ctx,
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

// rowScanner is the shared interface of *sql.Row and *sql.Rows (just Scan), so a
// single scan helper serves both a single-row Get and a list Query.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanAffiliate scans one affiliate row, decoding the JSON owners/hours columns.
func scanAffiliate(sc rowScanner) (*model.Affiliate, error) {
	var a model.Affiliate
	var ownersJSON, hoursJSON string
	if err := sc.Scan(&a.ID, &a.Name, &a.Subtitle, &ownersJSON, &a.Location, &a.Timezone,
		&hoursJSON, &a.Details, &a.Logo, &a.Screenshot,
		&a.EmbedColor, &a.DiscordLink, &a.CarrdLink, &a.SortOrder, &a.CreatedAt); err != nil {
		return nil, err
	}
	a.Owners = decodeJSONArray[string](ownersJSON)
	a.Hours = decodeJSONArray[model.AffiliateHour](hoursJSON)
	return &a, nil
}
