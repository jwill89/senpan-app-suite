-- Game presets: a name, the win patterns a game started from it picks (pattern_ids, a
-- JSON array of ids), its game details, and the automatic-draw controls it fills in
-- (auto_call and auto_interval, in seconds).

-- name: ListGamePresets :many
SELECT id, name, pattern_ids, game_details, auto_call, auto_interval, created_at
FROM game_presets ORDER BY name, id;

-- name: CreateGamePreset :one
INSERT INTO game_presets (name, pattern_ids, game_details, auto_call, auto_interval)
VALUES (?, ?, ?, ?, ?)
RETURNING id, name, pattern_ids, game_details, auto_call, auto_interval, created_at;

-- name: UpdateGamePreset :one
UPDATE game_presets SET name = ?, pattern_ids = ?, game_details = ?, auto_call = ?, auto_interval = ?
WHERE id = ?
RETURNING id, name, pattern_ids, game_details, auto_call, auto_interval, created_at;

-- name: DeleteGamePreset :execrows
DELETE FROM game_presets WHERE id = ?;
