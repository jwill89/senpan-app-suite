-- Passkeys: an account's WebAuthn credentials, each stored as go-webauthn's JSON
-- under its base64url credential id, with a name and when it was made and last used.

-- name: CreatePasskey :one
INSERT INTO user_passkeys (user_id, credential_id, credential, name) VALUES (?, ?, ?, ?)
RETURNING id, name, created_at, last_used_at;

-- name: ListPasskeys :many
SELECT id, name, created_at, last_used_at FROM user_passkeys WHERE user_id = ? ORDER BY created_at, id;

-- name: ListPasskeyCredentials :many
SELECT credential FROM user_passkeys WHERE user_id = ?;

-- name: PasskeyExists :one
SELECT EXISTS (SELECT 1 FROM user_passkeys WHERE credential_id = ?);

-- name: UsePasskey :exec
UPDATE user_passkeys SET credential = ?, last_used_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
WHERE credential_id = ?;

-- name: DeletePasskey :execrows
DELETE FROM user_passkeys WHERE id = ? AND user_id = ?;
