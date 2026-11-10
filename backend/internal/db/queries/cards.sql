-- Bingo cards: an id (six letters and digits), the board (board_data, its 5x5
-- numbers as compact row-major JSON, the form json.Marshal gives a [][]int, so the
-- same board is always the same text), the player it is issued to, staff's note
-- (details), whether Delete all spares it (protected), and a personal card's
-- lifecycle (custom_status: empty for a generated card, pending until staff approve
-- it, then approved) with its requester's world. The game checks the playable ones
-- for winners (ListPlayableCards).

-- name: ListCardSummaries :many
-- Every card without its board, by id: the Manage Cards list.
SELECT id, player_name, details, created_at, protected, custom_status, world
FROM cards ORDER BY id;

-- name: GetCard :one
SELECT id, board_data, player_name, details, created_at, protected, custom_status, world
FROM cards WHERE id = ?;

-- name: ListPlayableCards :many
-- Every card that can win a game: all but the personal cards still waiting for
-- staff to approve them.
SELECT id, board_data FROM cards WHERE custom_status != 'pending' ORDER BY id;

-- name: ListCardPlayerNames :many
-- The player each of these cards is issued to, for the winners log.
SELECT id, player_name FROM cards WHERE id IN (sqlc.slice('ids'));

-- name: CardExists :one
SELECT EXISTS (SELECT 1 FROM cards WHERE id = ?);

-- name: FindCardByBoard :one
-- A card whose numbers are exactly these, in the same cells.
SELECT id FROM cards WHERE board_data = ? LIMIT 1;

-- name: CreateCard :one
INSERT INTO cards (id, board_data, player_name) VALUES (?, ?, ?)
RETURNING id, board_data, player_name, details, created_at, protected, custom_status, world;

-- name: CreateCardRequest :exec
-- A personal card someone asked for: pending until staff approve it, and not
-- playable until then.
INSERT INTO cards (id, board_data, player_name, world, custom_status)
VALUES (?, ?, ?, ?, 'pending');

-- name: UpdateCard :one
-- Changes what is given (a null leaves it as it is).
UPDATE cards
SET player_name = COALESCE(sqlc.narg(player_name), player_name),
    details = COALESCE(sqlc.narg(details), details)
WHERE id = sqlc.arg(id)
RETURNING id, board_data, player_name, details, created_at, protected, custom_status, world;

-- name: ApproveCard :one
-- A pending personal card becomes playable, and protected.
UPDATE cards SET custom_status = 'approved', protected = 1
WHERE id = ? AND custom_status = 'pending'
RETURNING id, board_data, player_name, details, created_at, protected, custom_status, world;

-- name: SetCardProtected :one
UPDATE cards SET protected = ? WHERE id = ?
RETURNING id, board_data, player_name, details, created_at, protected, custom_status, world;

-- name: DeleteCard :execrows
DELETE FROM cards WHERE id = ?;

-- name: DeleteUnprotectedCards :many
-- Every card Delete all takes: the unprotected ones. Answers their ids.
DELETE FROM cards WHERE protected = 0 RETURNING id;
