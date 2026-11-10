-- Garapons: festival lottery drums. A garapon owns prize tiers, a drawing link per
-- player (a token and a draw allowance) and a log of the draws made. A draw keeps the
-- player's and the prize's names as they were, so the log survives both being removed.

-- name: ListGarapons :many
SELECT g.id, g.title, g.details, g.grand_prize_image, g.status, g.stamp_rally_id,
       CAST(COALESCE(sr.title, '') AS TEXT) AS stamp_rally_title, g.default_draws, g.created_at,
       CAST((SELECT COUNT(*) FROM garapon_players p WHERE p.garapon_id = g.id) AS INTEGER) AS player_count,
       CAST((SELECT COUNT(*) FROM garapon_draws d WHERE d.garapon_id = g.id) AS INTEGER) AS draw_count
FROM garapons g
LEFT JOIN stamp_rallies sr ON sr.id = g.stamp_rally_id
ORDER BY g.created_at DESC, g.id DESC;

-- name: GetGarapon :one
SELECT g.id, g.title, g.details, g.grand_prize_image, g.status, g.stamp_rally_id,
       CAST(COALESCE(sr.title, '') AS TEXT) AS stamp_rally_title, g.default_draws, g.created_at,
       CAST((SELECT COUNT(*) FROM garapon_players p WHERE p.garapon_id = g.id) AS INTEGER) AS player_count,
       CAST((SELECT COUNT(*) FROM garapon_draws d WHERE d.garapon_id = g.id) AS INTEGER) AS draw_count
FROM garapons g
LEFT JOIN stamp_rallies sr ON sr.id = g.stamp_rally_id
WHERE g.id = ?;

-- name: CreateGarapon :one
INSERT INTO garapons (title, details, grand_prize_image, status, stamp_rally_id, default_draws)
VALUES (?, ?, ?, 'open', ?, ?)
RETURNING id;

-- name: UpdateGarapon :execrows
-- Everything a garapon's form edits; its status is not.
UPDATE garapons
SET title = ?, details = ?, grand_prize_image = ?, stamp_rally_id = ?, default_draws = ?
WHERE id = ?;

-- name: DeleteGarapon :execrows
-- Its prizes, players and draws go with it (cascade).
DELETE FROM garapons WHERE id = ?;

-- name: SetGaraponStatus :execrows
UPDATE garapons SET status = ? WHERE id = ?;

-- name: ListGaraponPrizes :many
SELECT id, garapon_id, name, ball_color, rate, is_grand, sort_order
FROM garapon_prizes WHERE garapon_id = ? ORDER BY sort_order ASC, id ASC;

-- name: InsertGaraponPrize :exec
INSERT INTO garapon_prizes (garapon_id, name, ball_color, rate, is_grand, sort_order)
VALUES (?, ?, ?, ?, ?, ?);

-- name: DeleteGaraponPrizes :exec
DELETE FROM garapon_prizes WHERE garapon_id = ?;

-- name: GetStampRallyStatus :one
-- A rally's existence and whether it is open, for linking a garapon to it.
SELECT status FROM stamp_rallies WHERE id = ?;

-- name: ListGaraponPlayers :many
SELECT p.id, p.garapon_id, p.token, p.player_name, p.world, p.max_draws, p.created_at,
       CAST((SELECT COUNT(*) FROM garapon_draws d WHERE d.player_id = p.id) AS INTEGER) AS draws_used,
       CAST(COALESCE(sc.token, '') AS TEXT) AS stamp_card_token
FROM garapon_players p
LEFT JOIN stamp_rally_cards sc ON sc.id = p.stamp_card_id
WHERE p.garapon_id = ?
ORDER BY p.created_at ASC, p.id ASC;

-- name: GetGaraponPlayer :one
SELECT p.id, p.garapon_id, p.token, p.player_name, p.world, p.max_draws, p.created_at,
       CAST((SELECT COUNT(*) FROM garapon_draws d WHERE d.player_id = p.id) AS INTEGER) AS draws_used,
       CAST(COALESCE(sc.token, '') AS TEXT) AS stamp_card_token
FROM garapon_players p
LEFT JOIN stamp_rally_cards sc ON sc.id = p.stamp_card_id
WHERE p.id = ?;

-- name: GetGaraponPlayerByToken :one
SELECT p.id, p.garapon_id, p.token, p.player_name, p.world, p.max_draws, p.created_at,
       CAST((SELECT COUNT(*) FROM garapon_draws d WHERE d.player_id = p.id) AS INTEGER) AS draws_used,
       CAST(COALESCE(sc.token, '') AS TEXT) AS stamp_card_token
FROM garapon_players p
LEFT JOIN stamp_rally_cards sc ON sc.id = p.stamp_card_id
WHERE p.token = ?;

-- name: CreateGaraponPlayer :one
INSERT INTO garapon_players (garapon_id, token, player_name, world, max_draws)
VALUES (?, ?, ?, ?, ?)
RETURNING id;

-- name: SetPlayerStampCard :exec
-- Ties a drawing link to the stamp card issued with it.
UPDATE garapon_players SET stamp_card_id = ? WHERE id = ?;

-- name: GetPlayerStampCardID :one
SELECT stamp_card_id FROM garapon_players WHERE id = ?;

-- name: DeleteGaraponPlayer :execrows
-- A link that has drawn is kept unless the garapon is closed (the caller says which:
-- force). Its draws stay in the log, detached from it.
DELETE FROM garapon_players
WHERE garapon_players.id = sqlc.arg(player_id)
  AND (CAST(sqlc.arg(force) AS INTEGER) = 1
       OR NOT EXISTS (SELECT 1 FROM garapon_draws d WHERE d.player_id = garapon_players.id));

-- name: SweepPlayerStampCard :exec
-- Removes the card issued with a deleted link, by the rule the rally's own card delete
-- keeps: a card that has collected stamps stays until its rally is closed. Its log rows
-- stay either way.
DELETE FROM stamp_rally_cards
WHERE stamp_rally_cards.id = ?
  AND (NOT EXISTS (SELECT 1 FROM stamp_rally_collected col WHERE col.card_id = stamp_rally_cards.id)
       OR EXISTS (SELECT 1 FROM stamp_rallies sr WHERE sr.id = stamp_rally_cards.rally_id AND sr.status = 'closed'));

-- name: ListGaraponDraws :many
SELECT id, garapon_id, player_id, prize_id, player_name, prize_name, ball_color, drawn_at
FROM garapon_draws WHERE garapon_id = ? ORDER BY drawn_at DESC, id DESC;

-- name: ListPlayerGaraponDraws :many
SELECT id, garapon_id, player_id, prize_id, player_name, prize_name, ball_color, drawn_at
FROM garapon_draws WHERE player_id = ? ORDER BY drawn_at ASC, id ASC;

-- name: GetGaraponDraw :one
SELECT id, garapon_id, player_id, prize_id, player_name, prize_name, ball_color, drawn_at
FROM garapon_draws WHERE id = ?;

-- name: GetDrawContext :one
-- What a draw checks: the player's allowance and the garapon's status.
SELECT p.garapon_id, p.player_name, p.world, p.max_draws, g.status
FROM garapon_players p
JOIN garapons g ON g.id = p.garapon_id
WHERE p.id = ?;

-- name: CountPlayerDraws :one
SELECT CAST(COUNT(*) AS INTEGER) FROM garapon_draws WHERE player_id = ?;

-- name: InsertGaraponDraw :one
INSERT INTO garapon_draws (garapon_id, player_id, prize_id, player_name, prize_name, ball_color)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: OpenGaraponForRally :one
-- The oldest open garapon linked to a rally: the one a sign-up for the rally issues a
-- drawing link for.
SELECT id, title, default_draws FROM garapons
WHERE stamp_rally_id = ? AND status = 'open'
ORDER BY id LIMIT 1;

-- name: ClearGaraponRallyLink :exec
-- Unlinks the garapons of a rally that is going away (the link is not an enforced key).
UPDATE garapons SET stamp_rally_id = NULL WHERE stamp_rally_id = ?;
