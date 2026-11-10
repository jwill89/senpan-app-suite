-- Bingo games: one row per game (status active while it runs, ended after; at most
-- one is active), the numbers it called in order (called_numbers), the win patterns
-- it was started with, copied so a pattern edited or deleted later leaves it as it
-- was (game_patterns), and the cards that have won it so far (winners_cache, a JSON
-- array of card ids, kept as each draw computes it). The game itself is
-- internal/game; the settings, cards, patterns and winners log it reads and writes
-- are in their own files.

-- name: GetActiveGame :one
SELECT id, status, created_at, winners_cache
FROM games WHERE status = 'active' ORDER BY id DESC LIMIT 1;

-- name: EndActiveGames :execrows
UPDATE games SET status = 'ended' WHERE status = 'active';

-- name: EndGame :execrows
UPDATE games SET status = 'ended' WHERE id = ? AND status = 'active';

-- name: CreateGame :one
INSERT INTO games (status, winners_cache) VALUES ('active', '[]')
RETURNING id, created_at;

-- name: AddGamePattern :exec
INSERT INTO game_patterns (game_id, pattern_id, pattern_name, pattern_data)
VALUES (?, ?, ?, ?);

-- name: ListGamePatterns :many
SELECT pattern_id, pattern_name, pattern_data
FROM game_patterns WHERE game_id = ? ORDER BY pattern_id;

-- name: AddCalledNumber :exec
INSERT INTO called_numbers (game_id, number, call_order) VALUES (?, ?, ?);

-- name: ListCalledNumbers :many
SELECT number FROM called_numbers WHERE game_id = ? ORDER BY call_order;

-- name: SetWinnersCache :exec
UPDATE games SET winners_cache = ? WHERE id = ?;
