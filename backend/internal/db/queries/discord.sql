-- Signing in with Discord: each account's link to one Discord account (user_discord),
-- by the snowflake that never changes, with the names it last signed in under; and the
-- links an admin hands out to sign up or to link a Discord account (discord_invites),
-- kept by the SHA-256 hash of their token. A link answers once, until it expires.

-- name: GetDiscordLinkByDiscordID :one
SELECT user_id FROM user_discord WHERE discord_id = ?;

-- name: GetDiscordLink :one
SELECT user_id, discord_id, username, display_name, linked_at FROM user_discord WHERE user_id = ?;

-- name: ListDiscordLinks :many
SELECT user_id, discord_id, username, display_name, linked_at FROM user_discord;

-- name: LinkDiscord :exec
-- Links the account, replacing a Discord account it was linked to before. A Discord
-- account linked to another account is refused by the unique discord_id.
INSERT INTO user_discord (user_id, discord_id, username, display_name) VALUES (?, ?, ?, ?)
ON CONFLICT (user_id) DO UPDATE SET
    discord_id = excluded.discord_id,
    username = excluded.username,
    display_name = excluded.display_name,
    linked_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now');

-- name: RefreshDiscordNames :exec
UPDATE user_discord SET username = ?, display_name = ? WHERE discord_id = ?;

-- name: UnlinkDiscord :execrows
DELETE FROM user_discord WHERE user_id = ?;

-- name: CreateDiscordInvite :exec
INSERT INTO discord_invites (token_hash, kind, user_id, expires_at) VALUES (?, ?, ?, ?);

-- name: DeleteDiscordInvitesFor :exec
-- An account keeps one link of its own: a new one replaces the last.
DELETE FROM discord_invites WHERE user_id = ?;

-- name: DeleteExpiredDiscordInvites :exec
DELETE FROM discord_invites WHERE expires_at <= strftime('%Y-%m-%dT%H:%M:%SZ', 'now');

-- name: GetDiscordInvite :one
SELECT id, kind, user_id FROM discord_invites
WHERE token_hash = ? AND expires_at > strftime('%Y-%m-%dT%H:%M:%SZ', 'now');

-- name: GetDiscordInviteByID :one
SELECT id, kind, user_id FROM discord_invites
WHERE id = ? AND expires_at > strftime('%Y-%m-%dT%H:%M:%SZ', 'now');

-- name: UseDiscordInvite :execrows
DELETE FROM discord_invites WHERE id = ? AND expires_at > strftime('%Y-%m-%dT%H:%M:%SZ', 'now');
