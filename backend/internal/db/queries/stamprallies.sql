-- Stamp rallies: events whose stamps participants collect on a card of their own by
-- entering a stall's password. A rally owns stamps and prizes (each placed on the card
-- image), a card per participant and a log of the stamps collected, which keeps the
-- participant's and the stall's names as they were so it outlives both.

-- name: ListStampRallies :many
SELECT r.id, r.title, r.card_image, r.not_stamped_image, r.available_from, r.available_to, r.details,
       r.redeem_instructions, r.redeem_image, r.status, r.public_signup, r.completion_mode, r.required_food,
       r.required_game, r.festival_map_id, CAST(COALESCE(fm.title, '') AS TEXT) AS festival_map_name, r.created_at,
       CAST((SELECT COUNT(*) FROM stamp_rally_cards c WHERE c.rally_id = r.id) AS INTEGER) AS card_count,
       CAST((SELECT COUNT(*) FROM stamp_rally_cards c WHERE c.rally_id = r.id AND c.completed = 1) AS INTEGER) AS completed_count,
       CAST((SELECT COUNT(*) FROM stamp_rally_stamps st WHERE st.rally_id = r.id) AS INTEGER) AS stamp_count,
       CAST((SELECT COUNT(*) FROM stamp_rally_stamps st WHERE st.rally_id = r.id AND st.paused = 0) AS INTEGER) AS active_stamp_count
FROM stamp_rallies r
LEFT JOIN festival_maps fm ON fm.id = r.festival_map_id
ORDER BY r.created_at DESC, r.id DESC;

-- name: GetStampRally :one
SELECT r.id, r.title, r.card_image, r.not_stamped_image, r.available_from, r.available_to, r.details,
       r.redeem_instructions, r.redeem_image, r.status, r.public_signup, r.completion_mode, r.required_food,
       r.required_game, r.festival_map_id, CAST(COALESCE(fm.title, '') AS TEXT) AS festival_map_name, r.created_at,
       CAST((SELECT COUNT(*) FROM stamp_rally_cards c WHERE c.rally_id = r.id) AS INTEGER) AS card_count,
       CAST((SELECT COUNT(*) FROM stamp_rally_cards c WHERE c.rally_id = r.id AND c.completed = 1) AS INTEGER) AS completed_count,
       CAST((SELECT COUNT(*) FROM stamp_rally_stamps st WHERE st.rally_id = r.id) AS INTEGER) AS stamp_count,
       CAST((SELECT COUNT(*) FROM stamp_rally_stamps st WHERE st.rally_id = r.id AND st.paused = 0) AS INTEGER) AS active_stamp_count
FROM stamp_rallies r
LEFT JOIN festival_maps fm ON fm.id = r.festival_map_id
WHERE r.id = ?;

-- name: CreateStampRally :one
INSERT INTO stamp_rallies (title, card_image, not_stamped_image, available_from, available_to, details,
                           redeem_instructions, redeem_image, public_signup, completion_mode, required_food,
                           required_game, festival_map_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: UpdateStampRally :execrows
-- Everything a rally's form edits; its status is not.
UPDATE stamp_rallies
SET title = ?, card_image = ?, not_stamped_image = ?, available_from = ?, available_to = ?, details = ?,
    redeem_instructions = ?, redeem_image = ?, public_signup = ?, completion_mode = ?, required_food = ?,
    required_game = ?, festival_map_id = ?
WHERE id = ?;

-- name: DeleteStampRally :execrows
-- Its stamps, prizes, cards and log go with it (cascade).
DELETE FROM stamp_rallies WHERE id = ?;

-- name: SetStampRallyStatus :execrows
UPDATE stamp_rallies SET status = ? WHERE id = ?;

-- name: ListStampRallyStamps :many
-- A stamp's stall is its occupant's title when the rally is on a festival map and its
-- affiliate's name otherwise; both are empty for neither.
SELECT st.id, st.rally_id, st.occupant_id, CAST(COALESCE(fo.title, '') AS TEXT) AS stall_name,
       st.affiliate_id, CAST(COALESCE(a.name, '') AS TEXT) AS affiliate_name,
       st.image, st.password, st.stamp_type, st.pos_x, st.pos_y, st.width, st.height, st.rotation,
       st.active_from, st.active_to, st.paused, st.sort_order
FROM stamp_rally_stamps st
LEFT JOIN affiliates a ON a.id = st.affiliate_id
LEFT JOIN festival_stall_occupants fo ON fo.id = st.occupant_id
WHERE st.rally_id = ?
ORDER BY st.sort_order ASC, st.id ASC;

-- name: ListStampRallyPrizes :many
SELECT id, rally_id, name, image, pos_x, pos_y, width, height, rotation, sort_order
FROM stamp_rally_prizes WHERE rally_id = ? ORDER BY sort_order ASC, id ASC;

-- name: InsertStampRallyStamp :one
INSERT INTO stamp_rally_stamps (rally_id, occupant_id, affiliate_id, image, password, stamp_type, pos_x, pos_y,
                                width, height, rotation, active_from, active_to, paused, sort_order)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: UpdateStampRallyStamp :execrows
-- Scoped to the rally as well as the stamp, so an id from another rally changes nothing.
UPDATE stamp_rally_stamps
SET occupant_id = ?, affiliate_id = ?, image = ?, password = ?, stamp_type = ?, pos_x = ?, pos_y = ?, width = ?,
    height = ?, rotation = ?, active_from = ?, active_to = ?, paused = ?, sort_order = ?
WHERE id = ? AND rally_id = ?;

-- name: ListStampRallyStampIDs :many
SELECT id FROM stamp_rally_stamps WHERE rally_id = ?;

-- name: DeleteStampRallyStamp :exec
-- Its collected rows stay in the log, detached from it.
DELETE FROM stamp_rally_stamps WHERE id = ? AND rally_id = ?;

-- name: InsertStampRallyPrize :exec
INSERT INTO stamp_rally_prizes (rally_id, name, image, pos_x, pos_y, width, height, rotation, sort_order)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: DeleteStampRallyPrizes :exec
DELETE FROM stamp_rally_prizes WHERE rally_id = ?;

-- name: SetStampPaused :execrows
-- Scoped to the rally, so another rally's stamp can't be flipped.
UPDATE stamp_rally_stamps SET paused = ? WHERE id = ? AND rally_id = ?;

-- name: GetRallyStamp :one
SELECT st.id, st.rally_id, st.occupant_id, CAST(COALESCE(fo.title, '') AS TEXT) AS stall_name,
       st.affiliate_id, CAST(COALESCE(a.name, '') AS TEXT) AS affiliate_name,
       st.image, st.password, st.stamp_type, st.pos_x, st.pos_y, st.width, st.height, st.rotation,
       st.active_from, st.active_to, st.paused, st.sort_order
FROM stamp_rally_stamps st
LEFT JOIN affiliates a ON a.id = st.affiliate_id
LEFT JOIN festival_stall_occupants fo ON fo.id = st.occupant_id
WHERE st.id = ? AND st.rally_id = ?;

-- name: CreateRallyCard :one
INSERT INTO stamp_rally_cards (rally_id, token, participant_name, world)
VALUES (?, ?, ?, ?)
RETURNING id;

-- name: GetRallyCard :one
SELECT c.id, c.rally_id, c.token, c.participant_name, c.world, c.completed, c.completed_at, c.created_at,
       CAST((SELECT COUNT(*) FROM stamp_rally_collected col WHERE col.card_id = c.id) AS INTEGER) AS collected_count
FROM stamp_rally_cards c WHERE c.id = ?;

-- name: GetRallyCardByToken :one
SELECT c.id, c.rally_id, c.token, c.participant_name, c.world, c.completed, c.completed_at, c.created_at,
       CAST((SELECT COUNT(*) FROM stamp_rally_collected col WHERE col.card_id = c.id) AS INTEGER) AS collected_count
FROM stamp_rally_cards c WHERE c.token = ?;

-- name: ListRallyCards :many
SELECT c.id, c.rally_id, c.token, c.participant_name, c.world, c.completed, c.completed_at, c.created_at,
       CAST((SELECT COUNT(*) FROM stamp_rally_collected col WHERE col.card_id = c.id) AS INTEGER) AS collected_count
FROM stamp_rally_cards c WHERE c.rally_id = ?
ORDER BY c.created_at DESC, c.id DESC;

-- name: SetRallyCardCompleted :exec
UPDATE stamp_rally_cards SET completed = 1, completed_at = ? WHERE id = ?;

-- name: DeleteRallyCard :execrows
-- A card that has collected stamps is kept unless the rally is closed (the caller says
-- which: force). Its log rows stay either way.
DELETE FROM stamp_rally_cards
WHERE stamp_rally_cards.id = sqlc.arg(card_id) AND stamp_rally_cards.rally_id = sqlc.arg(rally_id)
  AND (CAST(sqlc.arg(force) AS INTEGER) = 1
       OR NOT EXISTS (SELECT 1 FROM stamp_rally_collected col WHERE col.card_id = stamp_rally_cards.id));

-- name: ListCollectedStamps :many
-- The stamps a card holds. A log row whose stamp was deleted has no stamp id and is left out.
SELECT stamp_id, stamped_at FROM stamp_rally_collected WHERE card_id = ? AND stamp_id IS NOT NULL;

-- name: CollectStamp :execrows
-- Records the participant's name, the stall's name and the stamp's type as they are
-- when the stamp is collected. A card holds a stamp once: the unique pair makes a
-- second collect insert nothing.
INSERT INTO stamp_rally_collected (rally_id, card_id, stamp_id, participant_name, stall_name, stamp_type)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (card_id, stamp_id) DO NOTHING;

-- name: ListRallyCollections :many
-- The log, a participant's rows together.
SELECT card_id, stamp_id, participant_name, stall_name, stamp_type, stamped_at
FROM stamp_rally_collected WHERE rally_id = ?
ORDER BY participant_name COLLATE NOCASE ASC, stamped_at ASC, id ASC;

-- name: ListSignupRallies :many
-- The rallies open to self sign-up, with the title of the open garapon linked to each.
-- The availability window is applied by the caller against its clock.
SELECT r.id, r.title, r.card_image, r.details, r.available_from, r.available_to,
       CAST(COALESCE((SELECT g.title FROM garapons g WHERE g.stamp_rally_id = r.id AND g.status = 'open'
                      ORDER BY g.id LIMIT 1), '') AS TEXT) AS garapon_title
FROM stamp_rallies r
WHERE r.status = 'open' AND r.public_signup = 1
ORDER BY r.created_at DESC, r.id DESC;

-- name: CountRallyCardsNamed :one
-- Whether a participant already holds a card in a rally: the whole name and world,
-- ignoring case.
SELECT CAST(COUNT(*) AS INTEGER) FROM stamp_rally_cards
WHERE rally_id = ? AND participant_name = sqlc.arg(name) COLLATE NOCASE AND world = sqlc.arg(world) COLLATE NOCASE;

-- name: CreateGaraponPlayerForCard :exec
-- A drawing link issued with a stamp card, on a token of its own.
INSERT INTO garapon_players (garapon_id, token, player_name, world, max_draws, stamp_card_id)
VALUES (?, ?, ?, ?, ?, ?);

-- name: LookUpParticipantCards :many
-- The cards held under a name and world, ignoring case, in rallies still open, with the
-- draws left on the drawing link issued with each. The drawing token is deliberately not
-- selected: this is keyed on a character name, which is public, and a draw can't be
-- undone. A card with no world (staff issued it, or the world was never known) is found
-- by the name alone, or by the whole "Name @ World" label.
SELECT r.id AS rally_id, r.title AS rally_title, c.token AS card_token, c.participant_name, c.world, c.completed,
       CAST(COALESCE(g.title, '') AS TEXT) AS garapon_title,
       CAST(COALESCE(gp.max_draws, 0) - COALESCE((SELECT COUNT(*) FROM garapon_draws d WHERE d.player_id = gp.id), 0) AS INTEGER) AS draws_left
FROM stamp_rally_cards c
JOIN stamp_rallies r ON r.id = c.rally_id
LEFT JOIN garapon_players gp ON gp.stamp_card_id = c.id
LEFT JOIN garapons g ON g.id = gp.garapon_id AND g.status = 'open'
WHERE r.status = 'open' AND (
    (c.participant_name = sqlc.arg(name) COLLATE NOCASE AND c.world = sqlc.arg(world) COLLATE NOCASE)
    OR (c.world = '' AND c.participant_name IN (sqlc.arg(name), sqlc.arg(label)) COLLATE NOCASE)
)
ORDER BY r.created_at DESC, r.id DESC;
