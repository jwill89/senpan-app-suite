-- Accounts: a username, an argon2id password hash, whether the account is an admin
-- and is active, its page permissions (a JSON array of keys), and its password
-- epoch, which a session must match. Every query that reads an account selects the
-- same columns, in the same order, and never the hash: only the two queries that
-- check or set a password touch it, so the hash reaches nothing else.

-- name: GetUser :one
SELECT id, username, is_admin, is_active, permissions, created_at, last_login_at, password_epoch
FROM users WHERE id = ?;

-- name: GetUserByUsername :one
SELECT id, username, is_admin, is_active, permissions, created_at, last_login_at, password_epoch
FROM users WHERE username = ?;

-- name: ListUsers :many
SELECT id, username, is_admin, is_active, permissions, created_at, last_login_at, password_epoch
FROM users ORDER BY username;

-- name: GetPasswordHash :one
SELECT password_hash FROM users WHERE id = ?;

-- name: CreateUser :exec
INSERT INTO users (username, password_hash) VALUES (?, ?);

-- name: UpdateUser :exec
UPDATE users SET is_active = ?, is_admin = ?, permissions = ? WHERE id = ?;

-- name: SetPassword :exec
UPDATE users SET password_hash = ?, password_epoch = password_epoch + 1 WHERE id = ?;

-- name: TouchLastLogin :exec
UPDATE users SET last_login_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = ?;
