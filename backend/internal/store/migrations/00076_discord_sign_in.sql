-- +goose Up
-- Signing in with Discord. user_discord links an account to one Discord account by its
-- snowflake, which never changes, and keeps the names it last signed in under for the
-- Users page. An account has at most one, and a Discord account links to one account.
CREATE TABLE user_discord (
  user_id INTEGER PRIMARY KEY,
  discord_id TEXT NOT NULL UNIQUE,
  username TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  linked_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (linked_at IS strftime('%Y-%m-%dT%H:%M:%SZ', linked_at)),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- An admin's link for someone to sign up with Discord (kind 'register', no account) or
-- to link their Discord to an existing account (kind 'link', that account's id). Only
-- a SHA-256 hash of the link's token is kept. A link answers once and expires; an
-- account has at most one link of its own waiting.
CREATE TABLE discord_invites (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  token_hash TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL CHECK (kind IN ('register', 'link')),
  user_id INTEGER,
  created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at)),
  expires_at DATETIME NOT NULL CHECK (expires_at IS strftime('%Y-%m-%dT%H:%M:%SZ', expires_at)),
  CHECK ((kind = 'link') = (user_id IS NOT NULL)),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_discord_invites_user ON discord_invites(user_id) WHERE user_id IS NOT NULL;
PRAGMA user_version = 76;
