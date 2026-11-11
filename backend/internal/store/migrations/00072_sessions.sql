-- +goose Up
-- Keeps sign-ins in the database, so a deploy or restart doesn't sign everyone out.
-- The table is scs's SQLite store's own shape: the token (a SHA-256 hash of the
-- cookie's), the gob-encoded session, and when it expires as a Julian day number,
-- which the store compares with julianday('now').
CREATE TABLE sessions (
  token TEXT PRIMARY KEY,
  data BLOB NOT NULL,
  expiry REAL NOT NULL
);
CREATE INDEX idx_sessions_expiry ON sessions(expiry);
PRAGMA user_version = 72;
