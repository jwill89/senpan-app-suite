package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"

	"app-suite/internal/model"
)

// ── Garapon operations ──────────────────────────────────────────────────────
//
// A garapon is a festival lottery drum (see model.Garapon). It owns prize tiers
// (each a ball color + appearance weight), tokenized per-player drawing links,
// and a draw log. Draws are server-authoritative: RecordGaraponDraw atomically
// re-checks the open status + remaining-draw cap, weighted-picks a prize, and
// logs it — so a player can never exceed their allowance or bias the odds.

// Sentinel errors returned by RecordGaraponDraw so the handler can map them to
// the right HTTP status (closed → 400, capped → 409, misconfigured → 400).
var (
	ErrGaraponClosed   = errors.New("garapon is closed")
	ErrGaraponNoDraws  = errors.New("no draws remaining")
	ErrGaraponNoPrizes = errors.New("garapon has no prizes")
)

// randToken returns an unguessable URL-safe token (16 random bytes, hex-encoded)
// for a player's private drawing link. Uses crypto/rand — these links are the
// only thing gating access to a draw, so they must not be predictable.
func randToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// prizeQueryer is the subset of *sql.DB / *sql.Tx used to read prize rows, so the
// same scan helper serves both the plain GetGarapon path and the in-transaction
// draw path.
type prizeQueryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// scanGaraponPrizes loads a garapon's prize tiers in display order from either a
// *sql.DB or an open *sql.Tx.
func scanGaraponPrizes(q prizeQueryer, garaponID int64) ([]model.GaraponPrize, error) {
	rows, err := q.Query(`SELECT id, garapon_id, name, ball_color, rate, is_grand, sort_order
		FROM garapon_prizes WHERE garapon_id = ? ORDER BY sort_order ASC, id ASC`, garaponID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prizes := make([]model.GaraponPrize, 0)
	for rows.Next() {
		var p model.GaraponPrize
		var grand int
		if err := rows.Scan(&p.ID, &p.GaraponID, &p.Name, &p.BallColor, &p.Rate, &grand, &p.SortOrder); err != nil {
			return nil, err
		}
		p.IsGrand = grand != 0
		prizes = append(prizes, p)
	}
	return prizes, rows.Err()
}

// insertGaraponPrizes writes a garapon's prize rows inside a transaction. The
// slice order becomes sort_order, so the admin's row ordering is preserved.
func insertGaraponPrizes(tx *sql.Tx, garaponID int64, prizes []model.GaraponPrize) error {
	for i, p := range prizes {
		grand := 0
		if p.IsGrand {
			grand = 1
		}
		if _, err := tx.Exec(`INSERT INTO garapon_prizes (garapon_id, name, ball_color, rate, is_grand, sort_order)
			VALUES (?, ?, ?, ?, ?, ?)`, garaponID, p.Name, p.BallColor, p.Rate, grand, i); err != nil {
			return err
		}
	}
	return nil
}

// CreateGarapon inserts a new garapon and its prizes in one transaction. The
// garapon is created "open" (instantly active). Returns the new garapon's ID.
func (s *Store) CreateGarapon(g *model.Garapon) (int64, error) {
	tx, err := s.beginImmediate()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`INSERT INTO garapons (title, details, grand_prize_image, status, stamp_rally_id)
		VALUES (?, ?, ?, 'open', ?)`, g.Title, g.Details, g.GrandPrizeImage, nullableID(g.StampRallyID))
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := insertGaraponPrizes(tx, id, g.Prizes); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// UpdateGarapon updates a garapon's editable fields and replaces its prize rows
// (delete + reinsert) in one transaction. Existing draws are unaffected — they
// snapshot the prize name/color and don't FK the prize row — so the draw log
// survives prize edits.
func (s *Store) UpdateGarapon(g *model.Garapon) error {
	tx, err := s.beginImmediate()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`UPDATE garapons SET title = ?, details = ?, grand_prize_image = ?, stamp_rally_id = ? WHERE id = ?`,
		g.Title, g.Details, g.GrandPrizeImage, nullableID(g.StampRallyID), g.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM garapon_prizes WHERE garapon_id = ?`, g.ID); err != nil {
		return err
	}
	if err := insertGaraponPrizes(tx, g.ID, g.Prizes); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteGarapon removes a garapon and (via ON DELETE CASCADE) its prizes,
// players, and draws. Returns true if a row was deleted.
func (s *Store) DeleteGarapon(id int64) (bool, error) {
	res, err := s.db.Exec("DELETE FROM garapons WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SetGaraponStatus updates a garapon's status ("open" or "closed").
func (s *Store) SetGaraponStatus(id int64, status string) error {
	_, err := s.db.Exec(`UPDATE garapons SET status = ? WHERE id = ?`, status, id)
	return err
}

// GetGarapon retrieves a single garapon with its prizes. Returns nil if not found.
func (s *Store) GetGarapon(id int64) (*model.Garapon, error) {
	var g model.Garapon
	var stampRallyID sql.NullInt64
	err := s.db.QueryRow(`SELECT g.id, g.title, g.details, g.grand_prize_image, g.status,
			g.stamp_rally_id, COALESCE(sr.title, ''), g.created_at
		FROM garapons g LEFT JOIN stamp_rallies sr ON sr.id = g.stamp_rally_id WHERE g.id = ?`, id).
		Scan(&g.ID, &g.Title, &g.Details, &g.GrandPrizeImage, &g.Status,
			&stampRallyID, &g.StampRallyTitle, &g.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if stampRallyID.Valid {
		g.StampRallyID = &stampRallyID.Int64
	}
	prizes, err := scanGaraponPrizes(s.db, id)
	if err != nil {
		return nil, err
	}
	g.Prizes = prizes
	return &g, nil
}

// ListGarapons returns every garapon (admin view), newest first, each carrying
// the read-only aggregates used by the list/closed tables: how many drawing
// links exist and how many draws have been made. Prizes are omitted here for
// efficiency (the detail fetch loads them).
func (s *Store) ListGarapons() ([]model.Garapon, error) {
	rows, err := s.db.Query(`SELECT g.id, g.title, g.details, g.grand_prize_image, g.status,
			g.stamp_rally_id, COALESCE(sr.title, ''), g.created_at,
			COALESCE((SELECT COUNT(*) FROM garapon_players p WHERE p.garapon_id = g.id), 0),
			COALESCE((SELECT COUNT(*) FROM garapon_draws d WHERE d.garapon_id = g.id), 0)
		FROM garapons g LEFT JOIN stamp_rallies sr ON sr.id = g.stamp_rally_id
		ORDER BY g.created_at DESC, g.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	garapons := make([]model.Garapon, 0)
	for rows.Next() {
		var g model.Garapon
		var stampRallyID sql.NullInt64
		if err := rows.Scan(&g.ID, &g.Title, &g.Details, &g.GrandPrizeImage, &g.Status,
			&stampRallyID, &g.StampRallyTitle, &g.CreatedAt,
			&g.PlayerCount, &g.DrawCount); err != nil {
			return nil, err
		}
		if stampRallyID.Valid {
			g.StampRallyID = &stampRallyID.Int64
		}
		garapons = append(garapons, g)
	}
	return garapons, rows.Err()
}

// ── Garapon players (tokenized drawing links) ───────────────────────────────

// ListGaraponPlayers returns a garapon's drawing links with each player's
// read-only draws-used count, oldest first.
func (s *Store) ListGaraponPlayers(garaponID int64) ([]model.GaraponPlayer, error) {
	rows, err := s.db.Query(`SELECT p.id, p.garapon_id, p.token, p.player_name, p.max_draws, p.created_at,
			COALESCE((SELECT COUNT(*) FROM garapon_draws d WHERE d.player_id = p.id), 0),
			COALESCE(sc.token, '')
		FROM garapon_players p LEFT JOIN stamp_rally_cards sc ON sc.id = p.stamp_card_id
		WHERE p.garapon_id = ? ORDER BY p.created_at ASC, p.id ASC`, garaponID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	players := make([]model.GaraponPlayer, 0)
	for rows.Next() {
		var p model.GaraponPlayer
		if err := rows.Scan(&p.ID, &p.GaraponID, &p.Token, &p.PlayerName, &p.MaxDraws, &p.CreatedAt, &p.DrawsUsed,
			&p.StampCardToken); err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	return players, rows.Err()
}

// getGaraponPlayer scans a single player row (with draws-used) by an arbitrary
// WHERE clause, shared by the by-id and by-token lookups.
func (s *Store) getGaraponPlayer(where string, arg any) (*model.GaraponPlayer, error) {
	var p model.GaraponPlayer
	err := s.db.QueryRow(`SELECT p.id, p.garapon_id, p.token, p.player_name, p.max_draws, p.created_at,
			COALESCE((SELECT COUNT(*) FROM garapon_draws d WHERE d.player_id = p.id), 0),
			COALESCE(sc.token, '')
		FROM garapon_players p LEFT JOIN stamp_rally_cards sc ON sc.id = p.stamp_card_id WHERE `+where, arg).
		Scan(&p.ID, &p.GaraponID, &p.Token, &p.PlayerName, &p.MaxDraws, &p.CreatedAt, &p.DrawsUsed,
			&p.StampCardToken)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetGaraponPlayerByID returns a single drawing link by id (nil if not found).
func (s *Store) GetGaraponPlayerByID(id int64) (*model.GaraponPlayer, error) {
	return s.getGaraponPlayer("p.id = ?", id)
}

// GetGaraponPlayerByToken returns a single drawing link by its token (nil if not
// found). This is the public player view's entry point.
func (s *Store) GetGaraponPlayerByToken(token string) (*model.GaraponPlayer, error) {
	return s.getGaraponPlayer("p.token = ?", token)
}

// CreateGaraponPlayer issues a new drawing link (a fresh unguessable token) for a
// named player with the given draw allowance, and returns it.
func (s *Store) CreateGaraponPlayer(garaponID int64, name string, maxDraws int) (*model.GaraponPlayer, error) {
	token, err := randToken()
	if err != nil {
		return nil, err
	}
	res, err := s.db.Exec(`INSERT INTO garapon_players (garapon_id, token, player_name, max_draws)
		VALUES (?, ?, ?, ?)`, garaponID, token, name, maxDraws)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetGaraponPlayerByID(id)
}

// SetPlayerStampCard links a drawing link to the Stamp Rally card auto-issued for it
// (so its token can be surfaced and the card cleaned up with the link).
func (s *Store) SetPlayerStampCard(playerID, cardID int64) error {
	_, err := s.db.Exec(`UPDATE garapon_players SET stamp_card_id = ? WHERE id = ?`, cardID, playerID)
	return err
}

// DeleteGaraponPlayer removes a drawing link. With force=false (garapon still
// open) it only deletes links that have not drawn yet — the NOT EXISTS guard.
// With force=true (garapon closed) it deletes regardless; the player's draws are
// KEPT in the log because garapon_draws.player_id is ON DELETE SET NULL (they
// detach rather than cascade-delete). Returns whether a row was deleted (false in
// the non-force case can also mean "blocked because the player has drawn").
func (s *Store) DeleteGaraponPlayer(playerID int64, force bool) (bool, error) {
	// Note the paired stamp card (if any) before deleting the link, so it can be
	// removed alongside (its stamp log is preserved — card_id ON DELETE SET NULL).
	var stampCardID sql.NullInt64
	_ = s.db.QueryRow(`SELECT stamp_card_id FROM garapon_players WHERE id = ?`, playerID).Scan(&stampCardID)

	var res sql.Result
	var err error
	if force {
		res, err = s.db.Exec(`DELETE FROM garapon_players WHERE id = ?`, playerID)
	} else {
		res, err = s.db.Exec(`DELETE FROM garapon_players WHERE id = ?
			AND NOT EXISTS (SELECT 1 FROM garapon_draws d WHERE d.player_id = ?)`, playerID, playerID)
	}
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 && stampCardID.Valid {
		// Remove the auto-issued stamp card too (best-effort); its collected rows stay
		// in the rally's log via the ON DELETE SET NULL + snapshots.
		_, _ = s.db.Exec(`DELETE FROM stamp_rally_cards WHERE id = ?`, stampCardID.Int64)
	}
	return n > 0, nil
}

// ── Garapon draws ───────────────────────────────────────────────────────────

// GetGaraponDraw returns a single draw row by id (nil if not found).
func (s *Store) GetGaraponDraw(id int64) (*model.GaraponDraw, error) {
	var d model.GaraponDraw
	var playerID sql.NullInt64 // null once the link is deleted (log is kept)
	err := s.db.QueryRow(`SELECT id, garapon_id, player_id, prize_id, player_name, prize_name, ball_color, drawn_at
		FROM garapon_draws WHERE id = ?`, id).
		Scan(&d.ID, &d.GaraponID, &playerID, &d.PrizeID, &d.PlayerName, &d.PrizeName, &d.BallColor, &d.DrawnAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.PlayerID = playerID.Int64
	return &d, nil
}

// scanGaraponDraws runs a draws query and scans the rows.
func (s *Store) scanGaraponDraws(query string, arg any) ([]model.GaraponDraw, error) {
	rows, err := s.db.Query(query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	draws := make([]model.GaraponDraw, 0)
	for rows.Next() {
		var d model.GaraponDraw
		var playerID sql.NullInt64 // null once the link is deleted (log is kept)
		if err := rows.Scan(&d.ID, &d.GaraponID, &playerID, &d.PrizeID, &d.PlayerName, &d.PrizeName, &d.BallColor, &d.DrawnAt); err != nil {
			return nil, err
		}
		d.PlayerID = playerID.Int64
		draws = append(draws, d)
	}
	return draws, rows.Err()
}

// ListGaraponDraws returns a garapon's full draw log, newest first (admin view).
func (s *Store) ListGaraponDraws(garaponID int64) ([]model.GaraponDraw, error) {
	return s.scanGaraponDraws(`SELECT id, garapon_id, player_id, prize_id, player_name, prize_name, ball_color, drawn_at
		FROM garapon_draws WHERE garapon_id = ? ORDER BY drawn_at DESC, id DESC`, garaponID)
}

// ListPlayerDraws returns one player's draws, oldest first (the public record).
func (s *Store) ListPlayerDraws(playerID int64) ([]model.GaraponDraw, error) {
	return s.scanGaraponDraws(`SELECT id, garapon_id, player_id, prize_id, player_name, prize_name, ball_color, drawn_at
		FROM garapon_draws WHERE player_id = ? ORDER BY drawn_at ASC, id ASC`, playerID)
}

// pickGaraponPrize chooses a prize weighted by appearance rate. Rates are relative
// weights (need not total 100); a prize with a non-positive rate is never picked
// unless every rate is non-positive, in which case the choice is uniform. Uses the
// shared randInt for a single source of randomness.
func pickGaraponPrize(prizes []model.GaraponPrize) *model.GaraponPrize {
	total := 0.0
	for i := range prizes {
		if prizes[i].Rate > 0 {
			total += prizes[i].Rate
		}
	}
	if total <= 0 {
		return &prizes[randInt(len(prizes))]
	}
	// A random float in [0, total) at 1e6 granularity (plenty for prize odds).
	r := float64(randInt(1_000_000)) / 1_000_000 * total
	cum := 0.0
	for i := range prizes {
		if prizes[i].Rate <= 0 {
			continue
		}
		cum += prizes[i].Rate
		if r < cum {
			return &prizes[i]
		}
	}
	// Floating-point fallback: the last prize with a positive rate.
	for i := len(prizes) - 1; i >= 0; i-- {
		if prizes[i].Rate > 0 {
			return &prizes[i]
		}
	}
	return &prizes[len(prizes)-1]
}

// RecordGaraponDraw performs one authoritative draw for a player, atomically:
// it re-verifies the garapon is open and the player has draws remaining, picks a
// weighted prize, and logs it (snapshotting the prize name + ball color). Returns
// the recorded draw, or ErrGaraponClosed / ErrGaraponNoDraws / ErrGaraponNoPrizes
// (and sql.ErrNoRows if the player vanished).
func (s *Store) RecordGaraponDraw(playerID int64) (*model.GaraponDraw, error) {
	tx, err := s.beginImmediate()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var garaponID int64
	var playerName, status string
	var maxDraws int
	err = tx.QueryRow(`SELECT p.garapon_id, p.player_name, p.max_draws, g.status
		FROM garapon_players p JOIN garapons g ON g.id = p.garapon_id WHERE p.id = ?`, playerID).
		Scan(&garaponID, &playerName, &maxDraws, &status)
	if err != nil {
		return nil, err // includes sql.ErrNoRows when the player is gone
	}
	if status != "open" {
		return nil, ErrGaraponClosed
	}

	var used int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM garapon_draws WHERE player_id = ?`, playerID).Scan(&used); err != nil {
		return nil, err
	}
	if used >= maxDraws {
		return nil, ErrGaraponNoDraws
	}

	prizes, err := scanGaraponPrizes(tx, garaponID)
	if err != nil {
		return nil, err
	}
	if len(prizes) == 0 {
		return nil, ErrGaraponNoPrizes
	}
	win := pickGaraponPrize(prizes)

	res, err := tx.Exec(`INSERT INTO garapon_draws (garapon_id, player_id, prize_id, player_name, prize_name, ball_color)
		VALUES (?, ?, ?, ?, ?, ?)`, garaponID, playerID, win.ID, playerName, win.Name, win.BallColor)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetGaraponDraw(id)
}
