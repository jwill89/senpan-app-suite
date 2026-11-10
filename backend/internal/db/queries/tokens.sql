-- Personal access tokens: at most one per account, kept as a SHA-256 hash with a
-- short prefix for the account page to show. The plaintext is never stored.

-- name: SetToken :one
INSERT INTO user_tokens (user_id, token_hash, token_prefix, created_at, last_used_at)
VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), NULL)
ON CONFLICT (user_id) DO UPDATE SET
    token_hash = excluded.token_hash,
    token_prefix = excluded.token_prefix,
    created_at = excluded.created_at,
    last_used_at = NULL
RETURNING token_prefix, created_at;

-- name: GetToken :one
SELECT token_prefix, created_at, last_used_at FROM user_tokens WHERE user_id = ?;

-- name: GetUserByToken :one
SELECT u.id, u.username, u.is_admin, u.is_active, u.permissions, u.created_at, u.last_login_at, u.password_epoch
FROM user_tokens t JOIN users u ON u.id = t.user_id
WHERE t.token_hash = ?;

-- name: TouchToken :exec
-- At most once a minute, so a busy client doesn't write on every request.
UPDATE user_tokens SET last_used_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
WHERE token_hash = ?
  AND (last_used_at IS NULL OR last_used_at <= strftime('%Y-%m-%dT%H:%M:%SZ', 'now', '-60 seconds'));

-- name: DeleteToken :execrows
DELETE FROM user_tokens WHERE user_id = ?;
