-- The settings table: one row per key, its value as text.

-- name: GetSetting :one
SELECT value FROM settings WHERE key = ?;

-- name: ListSettings :many
SELECT key, value FROM settings WHERE key IN (sqlc.slice('keys'));

-- name: SetSetting :exec
INSERT INTO settings (key, value) VALUES (?, ?)
ON CONFLICT (key) DO UPDATE SET value = excluded.value;
