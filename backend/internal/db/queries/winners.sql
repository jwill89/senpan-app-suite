-- The winners log: one row per winning card a game's end confirmed, with the player
-- named on the card then, the game's details, and its win patterns' names (a JSON
-- array). The game's end writes it (AddWinner, internal/game).

-- name: AddWinner :exec
INSERT INTO winners_log (card_id, player_name, game_details, winning_patterns)
VALUES (?, ?, ?, ?);

-- name: CountWinners :one
SELECT COUNT(*) FROM winners_log;

-- name: ListWinners :many
-- A page of the log, sorted by one of its columns (by the API's name for it, sort)
-- in a direction (dir, asc or desc), the id breaking ties the same way.
SELECT w.id, w.logged_at, w.card_id, w.player_name, w.game_details, w.winning_patterns
FROM winners_log w
JOIN (SELECT CAST(sqlc.arg(sort) AS TEXT) AS sort_key, CAST(sqlc.arg(dir) AS TEXT) AS sort_dir) o
ORDER BY
    CASE WHEN o.sort_dir = 'asc' THEN
        CASE o.sort_key
            WHEN 'cardId' THEN w.card_id
            WHEN 'playerName' THEN w.player_name
            WHEN 'gameDetails' THEN w.game_details
            ELSE w.logged_at
        END
    END ASC,
    CASE WHEN o.sort_dir = 'desc' THEN
        CASE o.sort_key
            WHEN 'cardId' THEN w.card_id
            WHEN 'playerName' THEN w.player_name
            WHEN 'gameDetails' THEN w.game_details
            ELSE w.logged_at
        END
    END DESC,
    CASE WHEN o.sort_dir = 'asc' THEN w.id END ASC,
    w.id DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(skip);

-- name: DeleteWinner :execrows
DELETE FROM winners_log WHERE id = ?;

-- name: DeleteAllWinners :execrows
DELETE FROM winners_log;

-- name: ListFrequentWinners :many
-- The players named on at least min_wins entries logged since a time.
SELECT player_name, COUNT(*) AS win_count
FROM winners_log
WHERE player_name != '' AND logged_at >= sqlc.arg(since)
GROUP BY player_name
HAVING COUNT(*) >= CAST(sqlc.arg(min_wins) AS INTEGER)
ORDER BY win_count DESC, player_name;
