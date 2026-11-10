-- Ema plaques: a festival's board takes plaques on its days (windows). The
-- one-plaque-per-person-per-day rule is a partial unique index, so two submissions
-- racing each other can't both land; a rejected plaque falls out of it.

-- name: ListEmaBoards :many
SELECT b.id, b.festival_map_id, CAST(COALESCE(m.title, '') AS TEXT) AS festival_map_name, b.title, b.template_image,
       b.instructions, b.discord_link, b.allow_self, b.allow_site, b.allow_artist, b.artist_name, b.status, b.created_at,
       CAST((SELECT COUNT(*) FROM ema_plaques p WHERE p.board_id = b.id AND p.status = 'pending') AS INTEGER) AS pending_count,
       CAST((SELECT COUNT(*) FROM ema_plaques p WHERE p.board_id = b.id AND p.status = 'requested') AS INTEGER) AS requested_count
FROM ema_boards b LEFT JOIN festival_maps m ON m.id = b.festival_map_id
ORDER BY b.created_at DESC, b.id DESC;

-- name: GetEmaBoard :one
SELECT b.id, b.festival_map_id, CAST(COALESCE(m.title, '') AS TEXT) AS festival_map_name, b.title, b.template_image,
       b.instructions, b.discord_link, b.allow_self, b.allow_site, b.allow_artist, b.artist_name, b.status, b.created_at,
       CAST((SELECT COUNT(*) FROM ema_plaques p WHERE p.board_id = b.id AND p.status = 'pending') AS INTEGER) AS pending_count,
       CAST((SELECT COUNT(*) FROM ema_plaques p WHERE p.board_id = b.id AND p.status = 'requested') AS INTEGER) AS requested_count
FROM ema_boards b LEFT JOIN festival_maps m ON m.id = b.festival_map_id
WHERE b.id = ?;

-- name: GetEmaBoardForMap :one
SELECT b.id, b.festival_map_id, CAST(COALESCE(m.title, '') AS TEXT) AS festival_map_name, b.title, b.template_image,
       b.instructions, b.discord_link, b.allow_self, b.allow_site, b.allow_artist, b.artist_name, b.status, b.created_at,
       CAST((SELECT COUNT(*) FROM ema_plaques p WHERE p.board_id = b.id AND p.status = 'pending') AS INTEGER) AS pending_count,
       CAST((SELECT COUNT(*) FROM ema_plaques p WHERE p.board_id = b.id AND p.status = 'requested') AS INTEGER) AS requested_count
FROM ema_boards b LEFT JOIN festival_maps m ON m.id = b.festival_map_id
WHERE b.festival_map_id = ?;

-- name: ListEmaWindows :many
SELECT w.id, w.board_id, w.label, w.start_at, w.end_at, w.webhook_url, w.artist_limit, w.sort_order,
       CAST((SELECT COUNT(*) FROM ema_plaques p WHERE p.window_id = w.id AND p.kind = 'artist') AS INTEGER) AS artist_count
FROM ema_windows w WHERE w.board_id = ? ORDER BY w.sort_order, w.start_at, w.id;

-- name: CreateEmaBoard :one
INSERT INTO ema_boards (festival_map_id, title, template_image, instructions, discord_link,
                        allow_self, allow_site, allow_artist, artist_name, status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'open')
RETURNING id;

-- name: UpdateEmaBoard :execrows
UPDATE ema_boards SET festival_map_id = ?, title = ?, template_image = ?, instructions = ?, discord_link = ?,
       allow_self = ?, allow_site = ?, allow_artist = ?, artist_name = ?
WHERE id = ?;

-- name: SetEmaBoardStatus :execrows
UPDATE ema_boards SET status = ? WHERE id = ?;

-- name: DeleteEmaBoard :execrows
DELETE FROM ema_boards WHERE id = ?;

-- name: ListEmaBoardImages :many
-- The plaque images a board still holds, to remove from disk once it is gone.
SELECT image FROM ema_plaques WHERE board_id = ? AND image != '';

-- name: ListEmaMapImages :many
-- The plaque images the board of a festival still holds.
SELECT p.image FROM ema_plaques p JOIN ema_boards b ON b.id = p.board_id
WHERE b.festival_map_id = ? AND p.image != '';

-- name: ListEmaWindowIDs :many
SELECT id FROM ema_windows WHERE board_id = ?;

-- name: InsertEmaWindow :exec
INSERT INTO ema_windows (board_id, label, start_at, end_at, webhook_url, artist_limit, sort_order)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpdateEmaWindow :execrows
UPDATE ema_windows SET label = ?, start_at = ?, end_at = ?, webhook_url = ?, artist_limit = ?, sort_order = ?
WHERE id = ? AND board_id = ?;

-- A deleted day's plaques keep their label; their window id clears.
-- name: DeleteEmaWindow :exec
DELETE FROM ema_windows WHERE id = ?;

-- name: ListEmaPlaques :many
SELECT p.id, p.board_id, p.window_id, p.window_label, p.kind, p.character_name, p.world, p.description, p.image,
       p.status, p.reviewed_by, p.reviewed_at, p.created_at, CAST(COALESCE(m.title, '') AS TEXT) AS festival_map_name
FROM ema_plaques p
JOIN ema_boards b ON b.id = p.board_id
LEFT JOIN festival_maps m ON m.id = b.festival_map_id
WHERE p.board_id = ?
ORDER BY p.created_at DESC, p.id DESC;

-- name: ListEmaArtistRequests :many
SELECT p.id, p.board_id, p.window_id, p.window_label, p.kind, p.character_name, p.world, p.description, p.image,
       p.status, p.reviewed_by, p.reviewed_at, p.created_at, CAST(COALESCE(m.title, '') AS TEXT) AS festival_map_name
FROM ema_plaques p
JOIN ema_boards b ON b.id = p.board_id
LEFT JOIN festival_maps m ON m.id = b.festival_map_id
WHERE p.kind = 'artist'
ORDER BY p.created_at DESC, p.id DESC;

-- name: ListEmaPlaquesFor :many
-- One visitor's plaques on a board, on the whole name and world, ignoring case: the
-- identity the one-per-day rule uses.
SELECT p.id, p.board_id, p.window_id, p.window_label, p.kind, p.character_name, p.world, p.description, p.image,
       p.status, p.reviewed_by, p.reviewed_at, p.created_at, CAST(COALESCE(m.title, '') AS TEXT) AS festival_map_name
FROM ema_plaques p
JOIN ema_boards b ON b.id = p.board_id
LEFT JOIN festival_maps m ON m.id = b.festival_map_id
WHERE p.board_id = sqlc.arg(board_id)
  AND p.character_name = sqlc.arg(name) COLLATE NOCASE AND p.world = sqlc.arg(world) COLLATE NOCASE
ORDER BY p.created_at DESC, p.id DESC;

-- name: GetEmaPlaque :one
SELECT p.id, p.board_id, p.window_id, p.window_label, p.kind, p.character_name, p.world, p.description, p.image,
       p.status, p.reviewed_by, p.reviewed_at, p.created_at, CAST(COALESCE(m.title, '') AS TEXT) AS festival_map_name
FROM ema_plaques p
JOIN ema_boards b ON b.id = p.board_id
LEFT JOIN festival_maps m ON m.id = b.festival_map_id
WHERE p.id = ?;

-- name: InsertEmaPlaque :one
INSERT INTO ema_plaques (board_id, window_id, window_label, kind, character_name, world, description, image, status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: GetEmaWindowLoad :one
-- A day's artist request limit and the requests it holds.
SELECT w.artist_limit,
       CAST((SELECT COUNT(*) FROM ema_plaques p WHERE p.window_id = w.id AND p.kind = 'artist') AS INTEGER) AS artist_count
FROM ema_windows w WHERE w.id = ?;

-- name: MoveEmaPlaque :execrows
-- The claim a review or a post takes (pending or requested -> posting), and its
-- release when the post fails: it applies only while the plaque is still in `from`.
UPDATE ema_plaques SET status = sqlc.arg(to_status) WHERE id = sqlc.arg(id) AND status = sqlc.arg(from_status);

-- name: FinishEmaPlaque :execrows
-- A plaque's final status, with who did it. Its image is cleared: Discord holds the
-- posted copy, and a rejected one is never used.
UPDATE ema_plaques SET status = sqlc.arg(to_status), image = '', reviewed_by = sqlc.arg(reviewed_by),
       reviewed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
WHERE id = sqlc.arg(id) AND status = sqlc.arg(from_status);

-- name: CompleteEmaRequest :execrows
UPDATE ema_plaques SET status = 'complete', reviewed_by = ?, reviewed_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
WHERE id = ? AND kind = 'artist' AND status = 'requested';

-- name: ReopenEmaRequest :execrows
UPDATE ema_plaques SET status = 'requested', reviewed_by = '', reviewed_at = NULL
WHERE id = ? AND kind = 'artist' AND status = 'complete';
