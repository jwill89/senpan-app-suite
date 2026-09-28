package store

import (
	"database/sql"
	"errors"

	"app-suite/internal/model"
)

// -- Stamp Rally --------------------------------------------------------------
//
// A Stamp Rally (see model.StampRally) is an event owning stamps + prizes (each
// with a placement on the card image), tokenized per-participant cards, and a
// collected-stamp log. This layer is pure data access; the time-window
// availability + completion business logic lives in server/stamprally.go (which
// loads these rows and computes against time.Now), mirroring how raffles keep
// availability checks in the server layer.

// ErrStampAlreadyCollected is returned by CollectStamp when the (card, stamp) pair
// already exists (the UNIQUE constraint) - a stamp can't be collected twice.
var ErrStampAlreadyCollected = errors.New("stamp already collected")

// -- Events -------------------------------------------------------------------

// ListStampRallies returns every rally, newest first, each with its issued-card
// and completed-card counts. Stamps/prizes are omitted (use GetStampRally).
func (s *Store) ListStampRallies() ([]model.StampRally, error) {
	rows, err := s.db.Query(`SELECT r.id, r.title, r.card_image, r.not_stamped_image,
			r.available_from, r.available_to, r.details, r.redeem_instructions, r.redeem_image, r.status,
			r.public_signup, r.completion_mode, r.required_food, r.required_game,
			r.festival_map_id, COALESCE(fm.title, ''), r.created_at,
			COALESCE((SELECT COUNT(*) FROM stamp_rally_cards c WHERE c.rally_id = r.id), 0),
			COALESCE((SELECT COUNT(*) FROM stamp_rally_cards c WHERE c.rally_id = r.id AND c.completed = 1), 0),
			COALESCE((SELECT COUNT(*) FROM stamp_rally_stamps st WHERE st.rally_id = r.id), 0),
			COALESCE((SELECT COUNT(*) FROM stamp_rally_stamps st WHERE st.rally_id = r.id AND st.paused = 0), 0)
		FROM stamp_rallies r LEFT JOIN festival_maps fm ON fm.id = r.festival_map_id
		ORDER BY r.created_at DESC, r.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rallies := make([]model.StampRally, 0)
	for rows.Next() {
		var r model.StampRally
		var publicSignup int
		var mapID sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Title, &r.CardImage, &r.NotStampedImage,
			&r.AvailableFrom, &r.AvailableTo, &r.Details, &r.RedeemInstructions, &r.RedeemImage, &r.Status,
			&publicSignup, &r.CompletionMode, &r.RequiredFood, &r.RequiredGame,
			&mapID, &r.FestivalMapName, &r.CreatedAt,
			&r.CardCount, &r.CompletedCount, &r.StampCount, &r.ActiveStampCount); err != nil {
			return nil, err
		}
		r.PublicSignup = publicSignup == 1
		if mapID.Valid {
			linked := mapID.Int64
			r.FestivalMapID = &linked
		}
		r.CompletionMode = model.NormalizeRallyCompletion(r.CompletionMode)
		rallies = append(rallies, r)
	}
	return rallies, rows.Err()
}

// GetStampRally returns a single rally with its stamps (affiliate name joined) and
// prizes. Returns nil if not found.
func (s *Store) GetStampRally(id int64) (*model.StampRally, error) {
	var r model.StampRally
	var publicSignup int
	var mapID sql.NullInt64
	err := s.db.QueryRow(`SELECT r.id, r.title, r.card_image, r.not_stamped_image,
			r.available_from, r.available_to, r.details, r.redeem_instructions, r.redeem_image, r.status,
			r.public_signup, r.completion_mode, r.required_food, r.required_game,
			r.festival_map_id, COALESCE(fm.title, ''), r.created_at
		FROM stamp_rallies r LEFT JOIN festival_maps fm ON fm.id = r.festival_map_id
		WHERE r.id = ?`, id).
		Scan(&r.ID, &r.Title, &r.CardImage, &r.NotStampedImage,
			&r.AvailableFrom, &r.AvailableTo, &r.Details, &r.RedeemInstructions, &r.RedeemImage, &r.Status,
			&publicSignup, &r.CompletionMode, &r.RequiredFood, &r.RequiredGame,
			&mapID, &r.FestivalMapName, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.PublicSignup = publicSignup == 1
	if mapID.Valid {
		linked := mapID.Int64
		r.FestivalMapID = &linked
	}
	r.CompletionMode = model.NormalizeRallyCompletion(r.CompletionMode)
	stamps, err := s.listStampRallyStamps(id)
	if err != nil {
		return nil, err
	}
	prizes, err := s.listStampRallyPrizes(id)
	if err != nil {
		return nil, err
	}
	r.Stamps = stamps
	r.Prizes = prizes
	return &r, nil
}

// listStampRallyStamps loads a rally's stamps in display order, joining both the
// festival-map stall title (empty when the rally isn't linked to a map) and the
// affiliate name (empty when the stamp has no affiliate -> the Senpan Tea House
// default). model.StampRallyStamp.DisplayStall picks between them.
func (s *Store) listStampRallyStamps(rallyID int64) ([]model.StampRallyStamp, error) {
	rows, err := s.db.Query(`SELECT st.id, st.rally_id, st.occupant_id, COALESCE(fo.title, ''),
			st.affiliate_id, COALESCE(a.name, ''),
			st.image, st.password, st.stamp_type, st.pos_x, st.pos_y, st.width, st.height, st.rotation,
			st.active_from, st.active_to, st.paused, st.sort_order
		FROM stamp_rally_stamps st
		LEFT JOIN affiliates a ON a.id = st.affiliate_id
		LEFT JOIN festival_stall_occupants fo ON fo.id = st.occupant_id
		WHERE st.rally_id = ? ORDER BY st.sort_order ASC, st.id ASC`, rallyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stamps := make([]model.StampRallyStamp, 0)
	for rows.Next() {
		var st model.StampRallyStamp
		var affiliateID, occupantID sql.NullInt64
		var paused int
		if err := rows.Scan(&st.ID, &st.RallyID, &occupantID, &st.StallName,
			&affiliateID, &st.AffiliateName,
			&st.Image, &st.Password, &st.StampType, &st.X, &st.Y, &st.Width, &st.Height, &st.Rotation,
			&st.ActiveFrom, &st.ActiveTo, &paused, &st.SortOrder); err != nil {
			return nil, err
		}
		if affiliateID.Valid {
			id := affiliateID.Int64
			st.AffiliateID = &id
		}
		if occupantID.Valid {
			id := occupantID.Int64
			st.OccupantID = &id
		}
		st.Paused = paused != 0
		st.StampType = model.NormalizeStampType(st.StampType)
		stamps = append(stamps, st)
	}
	return stamps, rows.Err()
}

// listStampRallyPrizes loads a rally's prizes in display order.
func (s *Store) listStampRallyPrizes(rallyID int64) ([]model.StampRallyPrize, error) {
	rows, err := s.db.Query(`SELECT id, rally_id, name, image, pos_x, pos_y, width, height, rotation, sort_order
		FROM stamp_rally_prizes WHERE rally_id = ? ORDER BY sort_order ASC, id ASC`, rallyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prizes := make([]model.StampRallyPrize, 0)
	for rows.Next() {
		var p model.StampRallyPrize
		if err := rows.Scan(&p.ID, &p.RallyID, &p.Name, &p.Image,
			&p.X, &p.Y, &p.Width, &p.Height, &p.Rotation, &p.SortOrder); err != nil {
			return nil, err
		}
		prizes = append(prizes, p)
	}
	return prizes, rows.Err()
}

// CreateStampRally inserts a new rally and its stamps + prizes in one transaction.
// Returns the new rally's ID.
func (s *Store) CreateStampRally(r *model.StampRally) (int64, error) {
	tx, err := s.beginImmediate()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`INSERT INTO stamp_rallies
			(title, card_image, not_stamped_image, available_from, available_to, details, redeem_instructions, redeem_image,
			public_signup, completion_mode, required_food, required_game, festival_map_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Title, r.CardImage, r.NotStampedImage, r.AvailableFrom, r.AvailableTo, r.Details, r.RedeemInstructions, r.RedeemImage,
		boolToInt(r.PublicSignup), r.CompletionMode, r.RequiredFood, r.RequiredGame, nullableID(r.FestivalMapID))
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, st := range r.Stamps {
		if _, err := insertStampRallyStamp(tx, id, st, i); err != nil {
			return 0, err
		}
	}
	for i, p := range r.Prizes {
		if err := insertStampRallyPrize(tx, id, p, i); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// UpdateStampRally updates a rally's editable fields and reconciles its stamps and
// prizes in one transaction. Stamps are UPSERTED by id (existing ids updated, id==0
// inserted, omitted ids deleted) so that collected-stamp history survives edits;
// prizes (which nothing references) are simply replaced.
// replaceStamps and replacePrizes say whether this save is authoritative for each
// child collection. False leaves that collection exactly as it is.
//
// The distinction matters because these are FULL-REPLACE collections: whatever the
// save carries becomes the whole set, and anything missing is deleted. That is
// correct when the client really is sending the complete list, and catastrophic
// when it isn't - an admin screen that opened its editor before the detail fetch
// landed (or after it failed) holds a list row whose stamps/prizes are simply
// absent, and saving it deleted every stamp on the rally along with every
// participant's collected rows. "The request didn't mention them" and "the request
// wants none of them" have to be different answers, so the handler decides from
// whether the JSON key was present at all and passes it down here.
func (s *Store) UpdateStampRally(r *model.StampRally, replaceStamps, replacePrizes bool) error {
	tx, err := s.beginImmediate()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`UPDATE stamp_rallies SET title = ?, card_image = ?, not_stamped_image = ?,
			available_from = ?, available_to = ?, details = ?, redeem_instructions = ?, redeem_image = ?,
			public_signup = ?, completion_mode = ?, required_food = ?, required_game = ?,
			festival_map_id = ? WHERE id = ?`,
		r.Title, r.CardImage, r.NotStampedImage, r.AvailableFrom, r.AvailableTo,
		r.Details, r.RedeemInstructions, r.RedeemImage, boolToInt(r.PublicSignup),
		r.CompletionMode, r.RequiredFood, r.RequiredGame, nullableID(r.FestivalMapID), r.ID); err != nil {
		return err
	}

	if replaceStamps {
		if err := reconcileRallyStamps(tx, r); err != nil {
			return err
		}
	}
	if replacePrizes {
		// Prizes have no per-user references -> replace wholesale.
		if _, err := tx.Exec(`DELETE FROM stamp_rally_prizes WHERE rally_id = ?`, r.ID); err != nil {
			return err
		}
		for i, p := range r.Prizes {
			if err := insertStampRallyPrize(tx, r.ID, p, i); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// reconcileRallyStamps makes the rally's stamps match r.Stamps exactly: an
// existing id is updated in place (so its collected history survives), id==0 is
// inserted, and anything left out is deleted along with its collected rows.
func reconcileRallyStamps(tx *sql.Tx, r *model.StampRally) error {
	keep := make(map[int64]bool)
	for i, st := range r.Stamps {
		if st.ID > 0 {
			if err := updateStampRallyStamp(tx, r.ID, st, i); err != nil {
				return err
			}
			keep[st.ID] = true
		} else {
			newID, err := insertStampRallyStamp(tx, r.ID, st, i)
			if err != nil {
				return err
			}
			keep[newID] = true
		}
	}
	// Delete stamps no longer present (their collected rows cascade away).
	existing, err := tx.Query(`SELECT id FROM stamp_rally_stamps WHERE rally_id = ?`, r.ID)
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
	// A read that stopped early is not the same as a short list: without this the
	// truncation reads as "these are all the stamps", and the ones never seen are
	// silently kept when the save meant to delete them.
	if err := existing.Err(); err != nil {
		return err
	}
	for _, id := range toDelete {
		if _, err := tx.Exec(`DELETE FROM stamp_rally_stamps WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

// insertStampRallyStamp inserts one stamp at the given sort order, returning its ID.
func insertStampRallyStamp(tx *sql.Tx, rallyID int64, st model.StampRallyStamp, sortOrder int) (int64, error) {
	res, err := tx.Exec(`INSERT INTO stamp_rally_stamps
			(rally_id, occupant_id, affiliate_id, image, password, stamp_type, pos_x, pos_y, width, height, rotation,
			active_from, active_to, paused, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rallyID, nullableID(st.OccupantID), nullableID(st.AffiliateID), st.Image, st.Password, st.StampType,
		st.X, st.Y, st.Width, st.Height, st.Rotation, st.ActiveFrom, st.ActiveTo, boolToInt(st.Paused), sortOrder)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// updateStampRallyStamp updates an existing stamp's editable fields + sort order.
// The WHERE clause is scoped to rallyID as well as the stamp id so a spoofed stamp
// id belonging to a different event can't be updated across rallies.
func updateStampRallyStamp(tx *sql.Tx, rallyID int64, st model.StampRallyStamp, sortOrder int) error {
	_, err := tx.Exec(`UPDATE stamp_rally_stamps SET occupant_id = ?, affiliate_id = ?, image = ?, password = ?, stamp_type = ?,
			pos_x = ?, pos_y = ?, width = ?, height = ?, rotation = ?,
			active_from = ?, active_to = ?, paused = ?, sort_order = ? WHERE id = ? AND rally_id = ?`,
		nullableID(st.OccupantID), nullableID(st.AffiliateID), st.Image, st.Password, st.StampType,
		st.X, st.Y, st.Width, st.Height, st.Rotation, st.ActiveFrom, st.ActiveTo, boolToInt(st.Paused), sortOrder, st.ID, rallyID)
	return err
}

// insertStampRallyPrize inserts one prize at the given sort order.
func insertStampRallyPrize(tx *sql.Tx, rallyID int64, p model.StampRallyPrize, sortOrder int) error {
	_, err := tx.Exec(`INSERT INTO stamp_rally_prizes
			(rally_id, name, image, pos_x, pos_y, width, height, rotation, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rallyID, p.Name, p.Image, p.X, p.Y, p.Width, p.Height, p.Rotation, sortOrder)
	return err
}

// nullableID maps an optional affiliate id pointer to a SQL-friendly value.
func nullableID(id *int64) any {
	if id == nil {
		return nil
	}
	return *id
}

// DeleteStampRally removes a rally and (via ON DELETE CASCADE) its stamps, prizes,
// cards, and collected log. It first clears the link on any garapon that referenced
// it (garapons.stamp_rally_id is a plain column, not an enforced FK). Returns true if
// a row was deleted.
func (s *Store) DeleteStampRally(id int64) (bool, error) {
	tx, err := s.beginImmediate()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`UPDATE garapons SET stamp_rally_id = NULL WHERE stamp_rally_id = ?`, id); err != nil {
		return false, err
	}
	res, err := tx.Exec(`DELETE FROM stamp_rallies WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return n > 0, nil
}

// SetStampRallyStatus updates a rally's manual status ("open" or "closed").
func (s *Store) SetStampRallyStatus(id int64, status string) error {
	_, err := s.db.Exec(`UPDATE stamp_rallies SET status = ? WHERE id = ?`, status, id)
	return err
}

// SetStampPaused toggles a single stamp's pause flag (a quick availability control
// without re-saving the whole event). It is scoped to rallyID so a caller can't
// flip a stamp belonging to a different rally.
func (s *Store) SetStampPaused(rallyID, stampID int64, paused bool) (bool, error) {
	res, err := s.db.Exec(`UPDATE stamp_rally_stamps SET paused = ? WHERE id = ? AND rally_id = ?`,
		boolToInt(paused), stampID, rallyID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// -- Participant cards (tokenized) --------------------------------------------

// IssueRallyCard creates a tokenized card for a named participant (fresh token) and
// returns it. World may be "" - staff issuing a card by hand do not always know it.
func (s *Store) IssueRallyCard(rallyID int64, name, world string) (*model.StampRallyCard, error) {
	token, err := randToken()
	if err != nil {
		return nil, err
	}
	return s.IssueRallyCardWithToken(rallyID, name, world, token)
}

// IssueRallyCardWithToken creates a card using a SUPPLIED token, for a caller that
// needs to know the token before the row exists.
func (s *Store) IssueRallyCardWithToken(rallyID int64, name, world, token string) (*model.StampRallyCard, error) {
	res, err := s.db.Exec(`INSERT INTO stamp_rally_cards (rally_id, token, participant_name, world)
		VALUES (?, ?, ?, ?)`, rallyID, token, name, world)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetRallyCardByID(id)
}

// -- Public self-service sign-up ----------------------------------------------

// ErrParticipantNameTaken is returned by SignUpForRally when the rally already has
// a card under that name. Names are compared case-insensitively, so "Yao Ming" and
// "yao ming" collide - one participant, one card, whatever they type. (SQLite's
// NOCASE folds ASCII only; a name differing solely in the case of a non-ASCII
// letter would slip through, which no FFXIV character name does.)
var ErrParticipantNameTaken = errors.New("participant name already signed up")

// ListSignupRallies returns the rallies open to public self-sign-up (status open +
// the public_signup opt-in), newest first, each with the title of the open Garapon
// linked to it if there is one. The availability WINDOW is not applied here - the
// caller filters it against the current time, as the other public paths do.
func (s *Store) ListSignupRallies() ([]model.SignupRally, error) {
	rows, err := s.db.Query(`SELECT r.id, r.title, r.card_image, r.details,
			r.available_from, r.available_to,
			COALESCE((SELECT g.title FROM garapons g
				WHERE g.stamp_rally_id = r.id AND g.status = 'open' ORDER BY g.id LIMIT 1), '')
		FROM stamp_rallies r
		WHERE r.status = 'open' AND r.public_signup = 1
		ORDER BY r.created_at DESC, r.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rallies := make([]model.SignupRally, 0)
	for rows.Next() {
		var r model.SignupRally
		if err := rows.Scan(&r.ID, &r.Title, &r.CardImage, &r.Details,
			&r.AvailableFrom, &r.AvailableTo, &r.GaraponTitle); err != nil {
			return nil, err
		}
		rallies = append(rallies, r)
	}
	return rallies, rows.Err()
}

// SignUpForRally issues a participant their own card for a rally and, when
// garaponID is non-nil, a Garapon drawing link with its OWN separate token, which
// it returns alongside the card (empty when there is no garapon).
//
// The two tokens used to be one string, so a single hash served /garapon/<token>
// and /stamp-card/<token>. That made the card link itself a spendable capability:
// a screenshot of a stamp card, or a card link pasted in Discord, let anyone spend
// that participant's draws - which are irreversible - and it meant any path that
// recovered a card link handed over the draws with it. Viewing and spending are
// now separate secrets, so a card link can be shared freely.
//
// Cards issued BEFORE this change still share one token; they are left alone
// deliberately, so links already in participants' hands keep working. The overlap
// ends as those rallies close.
//
// The name check and both inserts run inside one immediate transaction. Checking
// outside it would let two sign-ups racing on the same name both read "free" and
// both insert - the whole point of the check is that the second one loses.
func (s *Store) SignUpForRally(rallyID int64, name, world string, garaponID *int64, maxDraws int) (*model.StampRallyCard, string, error) {
	tx, err := s.beginImmediate()
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = tx.Rollback() }()

	// Matched on name AND world. Two players who share a character name on
	// different worlds are two people, and before the world had its own column the
	// second of them was turned away as a duplicate.
	var taken int
	err = tx.QueryRow(`SELECT COUNT(*) FROM stamp_rally_cards
		WHERE rally_id = ? AND participant_name = ? COLLATE NOCASE
		  AND world = ? COLLATE NOCASE`, rallyID, name, world).Scan(&taken)
	if err != nil {
		return nil, "", err
	}
	if taken > 0 {
		return nil, "", ErrParticipantNameTaken
	}

	token, err := randToken()
	if err != nil {
		return nil, "", err
	}
	res, err := tx.Exec(`INSERT INTO stamp_rally_cards (rally_id, token, participant_name, world)
		VALUES (?, ?, ?, ?)`, rallyID, token, name, world)
	if err != nil {
		return nil, "", err
	}
	cardID, err := res.LastInsertId()
	if err != nil {
		return nil, "", err
	}

	var garaponToken string
	if garaponID != nil {
		if garaponToken, err = randToken(); err != nil {
			return nil, "", err
		}
		if _, err := tx.Exec(`INSERT INTO garapon_players (garapon_id, token, player_name, world, max_draws, stamp_card_id)
			VALUES (?, ?, ?, ?, ?, ?)`, *garaponID, garaponToken, name, world, maxDraws, cardID); err != nil {
			return nil, "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, "", err
	}
	card, err := s.GetRallyCardByID(cardID)
	if err != nil {
		return nil, "", err
	}
	return card, garaponToken, nil
}

// LookupParticipantCards returns the cards held under an exact participant name
// (case-insensitive, whole string - never a prefix or substring) across rallies
// that are still open, with the paired Garapon drawing link when its garapon is
// also open. An unknown name simply yields no rows: the caller returns that as an
// empty list, so the endpoint can't be used to probe who signed up for what.
func (s *Store) LookupParticipantCards(name, world string) ([]model.StampLookupEntry, error) {
	// gp.token is deliberately NOT selected. A draw is irreversible and the token is
	// the whole capability, so a lookup keyed on a character name - which is public,
	// and appears on the entrant lists the app publishes by design - must never
	// return one. What it reports instead is how many draws are LEFT, which tells a
	// participant on a borrowed device that their draws are intact without handing
	// anyone the means to spend them. The card token is still returned: it opens the
	// stamp card and, since the tokens were split, nothing else.
	rows, err := s.db.Query(`SELECT r.id, r.title, c.token, c.participant_name, c.world, c.completed,
			COALESCE(g.title, ''),
			COALESCE(gp.max_draws, 0) - COALESCE((
				SELECT COUNT(*) FROM garapon_draws d WHERE d.player_id = gp.id), 0)
		FROM stamp_rally_cards c
		JOIN stamp_rallies r ON r.id = c.rally_id
		LEFT JOIN garapon_players gp ON gp.stamp_card_id = c.id
		LEFT JOIN garapons g ON g.id = gp.garapon_id AND g.status = 'open'
		WHERE r.status = 'open' AND (
			(c.participant_name = ? COLLATE NOCASE AND c.world = ? COLLATE NOCASE)
			-- A card with no world is one the v66 backfill could not split (the
			-- participant never typed a world) or one staff issued without asking.
			-- It is still findable by the name alone, and by the whole label in case
			-- it kept the composed form.
			OR (c.world = '' AND c.participant_name IN (?, ?) COLLATE NOCASE)
		)
		ORDER BY r.created_at DESC, r.id DESC`,
		name, world, name, model.ParticipantLabel(name, world))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]model.StampLookupEntry, 0)
	for rows.Next() {
		var e model.StampLookupEntry
		var completed int
		if err := rows.Scan(&e.RallyID, &e.RallyTitle, &e.CardToken,
			&e.ParticipantName, &e.World, &completed,
			&e.GaraponTitle, &e.GaraponDrawsLeft); err != nil {
			return nil, err
		}
		// No garapon row joined, so the subtraction above is meaningless.
		if e.GaraponTitle == "" {
			e.GaraponDrawsLeft = 0
		}
		e.Completed = completed == 1
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// DeleteRallyCard removes a participant card, scoped to its rally. With force=false
// (rally open) it only deletes a card that has NO collected stamps (the NOT EXISTS
// guard). With force=true (rally closed) it deletes regardless; the card's collected
// rows are KEPT in the log because stamp_rally_collected.card_id is ON DELETE SET NULL
// (and the rows snapshot participant/stall). Returns whether a row was deleted (false
// in the non-force case can also mean "blocked because the card has stamps").
func (s *Store) DeleteRallyCard(rallyID, cardID int64, force bool) (bool, error) {
	var res sql.Result
	var err error
	if force {
		res, err = s.db.Exec(`DELETE FROM stamp_rally_cards WHERE id = ? AND rally_id = ?`, cardID, rallyID)
	} else {
		res, err = s.db.Exec(`DELETE FROM stamp_rally_cards WHERE id = ? AND rally_id = ?
			AND NOT EXISTS (SELECT 1 FROM stamp_rally_collected col WHERE col.card_id = ?)`, cardID, rallyID, cardID)
	}
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// scanRallyCard scans a single card row (with completed flag + nullable completed_at).
func scanRallyCard(sc rowScanner) (*model.StampRallyCard, error) {
	var c model.StampRallyCard
	var completed int
	var completedAt sql.NullString
	if err := sc.Scan(&c.ID, &c.RallyID, &c.Token, &c.ParticipantName, &c.World, &completed, &completedAt, &c.CreatedAt); err != nil {
		return nil, err
	}
	c.Completed = completed != 0
	c.CompletedAt = completedAt.String
	return &c, nil
}

// GetRallyCardByID returns a single card by id (nil if not found).
func (s *Store) GetRallyCardByID(id int64) (*model.StampRallyCard, error) {
	row := s.db.QueryRow(`SELECT id, rally_id, token, participant_name, world, completed, completed_at, created_at
		FROM stamp_rally_cards WHERE id = ?`, id)
	c, err := scanRallyCard(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return c, err
}

// GetRallyCardByToken returns a single card by its token (nil if not found). This is the
// public stamp-card view's entry point.
func (s *Store) GetRallyCardByToken(token string) (*model.StampRallyCard, error) {
	row := s.db.QueryRow(`SELECT id, rally_id, token, participant_name, world, completed, completed_at, created_at
		FROM stamp_rally_cards WHERE token = ?`, token)
	c, err := scanRallyCard(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return c, err
}

// ListRallyCards returns a rally's participant cards (newest first) with each card's
// collected-stamp count.
func (s *Store) ListRallyCards(rallyID int64) ([]model.StampRallyCard, error) {
	rows, err := s.db.Query(`SELECT c.id, c.rally_id, c.token, c.participant_name, c.world, c.completed, c.completed_at, c.created_at,
			COALESCE((SELECT COUNT(*) FROM stamp_rally_collected col WHERE col.card_id = c.id), 0)
		FROM stamp_rally_cards c WHERE c.rally_id = ? ORDER BY c.created_at DESC, c.id DESC`, rallyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cards := make([]model.StampRallyCard, 0)
	for rows.Next() {
		var c model.StampRallyCard
		var completed int
		var completedAt sql.NullString
		if err := rows.Scan(&c.ID, &c.RallyID, &c.Token, &c.ParticipantName, &c.World, &completed, &completedAt, &c.CreatedAt,
			&c.CollectedCount); err != nil {
			return nil, err
		}
		c.Completed = completed != 0
		c.CompletedAt = completedAt.String
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// SetRallyCardCompleted marks a card completed at the given timestamp (RFC-3339). Used by
// the lazy completion recompute when the last collectable stamp is accounted for.
func (s *Store) SetRallyCardCompleted(cardID int64, completedAt string) error {
	_, err := s.db.Exec(`UPDATE stamp_rally_cards SET completed = 1, completed_at = ? WHERE id = ?`,
		completedAt, cardID)
	return err
}

// -- Collected stamps ---------------------------------------------------------

// ListCollectedStampIDs returns the set of stamp ids a card has collected.
func (s *Store) ListCollectedStampIDs(cardID int64) (map[int64]string, error) {
	rows, err := s.db.Query(`SELECT stamp_id, stamped_at FROM stamp_rally_collected WHERE card_id = ?`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	collected := make(map[int64]string)
	for rows.Next() {
		// stamp_id is nullable (ON DELETE SET NULL, schema v41): a collected row
		// survives its stamp's removal with a NULL stamp_id. Skip those rather
		// than failing the scan (which would 500 the participant's card page).
		var stampID sql.NullInt64
		var stampedAt string
		if err := rows.Scan(&stampID, &stampedAt); err != nil {
			return nil, err
		}
		if stampID.Valid {
			collected[stampID.Int64] = stampedAt
		}
	}
	return collected, rows.Err()
}

// CollectStamp records that a card collected a stamp, snapshotting the participant
// name + stall name + stamp type (so the log survives card/stamp deletion) and the
// rally id (so it cascade-deletes only with the whole rally). Returns
// ErrStampAlreadyCollected when the (card, stamp) pair already exists (the UNIQUE guard).
func (s *Store) CollectStamp(rallyID, cardID, stampID int64, participantName, stallName, stampType string) (*model.StampRallyCollected, error) {
	res, err := s.db.Exec(`INSERT INTO stamp_rally_collected
			(rally_id, card_id, stamp_id, participant_name, stall_name, stamp_type) VALUES (?, ?, ?, ?, ?, ?)`,
		rallyID, cardID, stampID, participantName, stallName, stampType)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrStampAlreadyCollected
		}
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	var c model.StampRallyCollected
	var cardCol, stampCol sql.NullInt64
	err = s.db.QueryRow(`SELECT id, rally_id, card_id, stamp_id, participant_name, stall_name, stamp_type, stamped_at
		FROM stamp_rally_collected WHERE id = ?`, id).
		Scan(&c.ID, &c.RallyID, &cardCol, &stampCol, &c.ParticipantName, &c.StallName, &c.StampType, &c.StampedAt)
	if err != nil {
		return nil, err
	}
	c.CardID = cardCol.Int64
	c.StampID = stampCol.Int64
	return &c, nil
}

// ListRallyCollections returns the event-wide stamp log straight from the snapshot
// columns, so rows persist after their card or stamp is deleted. Ordered by participant
// then time so the admin table can group a participant's rows together by default.
func (s *Store) ListRallyCollections(rallyID int64) ([]model.StampRallyLogEntry, error) {
	rows, err := s.db.Query(`SELECT card_id, stamp_id, participant_name, stall_name, stamp_type, stamped_at
		FROM stamp_rally_collected WHERE rally_id = ?
		ORDER BY participant_name COLLATE NOCASE ASC, stamped_at ASC, id ASC`, rallyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]model.StampRallyLogEntry, 0)
	for rows.Next() {
		var e model.StampRallyLogEntry
		var cardID, stampID sql.NullInt64
		if err := rows.Scan(&cardID, &stampID, &e.ParticipantName, &e.StallName, &e.StampType, &e.StampedAt); err != nil {
			return nil, err
		}
		e.CardID = cardID.Int64
		e.StampID = stampID.Int64
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
