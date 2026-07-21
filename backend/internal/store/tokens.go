package store

import (
	"database/sql"
	"errors"

	"app-suite/internal/model"
)

// ── Personal access tokens ───────────────────────────────────────────────────
//
// Each account may hold at most one personal access token (user_id is the
// primary key of user_tokens), used by external API clients such as the FFXIV
// plugin. Only a SHA-256 hash of the token is stored; the plaintext is shown
// once at creation and never persisted, so a database leak cannot yield a usable
// token. The server layer owns token generation/hashing (see server/tokens.go);
// this layer is pure persistence.

// UpsertUserToken stores (or replaces) the access token for a user. A new
// hash/prefix overwrites any existing token — the old token stops working
// immediately — and resets created_at/last_used_at.
func (s *Store) UpsertUserToken(userID int64, tokenHash, prefix string) error {
	_, err := s.db.Exec(
		`INSERT INTO user_tokens (user_id, token_hash, token_prefix, created_at, last_used_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP, NULL)
		 ON CONFLICT(user_id) DO UPDATE SET
		     token_hash = excluded.token_hash,
		     token_prefix = excluded.token_prefix,
		     created_at = CURRENT_TIMESTAMP,
		     last_used_at = NULL`,
		userID, tokenHash, prefix,
	)
	return err
}

// GetUserTokenInfo returns the non-secret metadata for a user's token. HasToken
// is false (and the other fields empty) when the account has none.
func (s *Store) GetUserTokenInfo(userID int64) (model.TokenInfo, error) {
	var info model.TokenInfo
	err := s.db.QueryRow(
		`SELECT token_prefix, created_at, COALESCE(last_used_at, '')
		   FROM user_tokens WHERE user_id = ?`, userID).
		Scan(&info.Prefix, &info.CreatedAt, &info.LastUsedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.TokenInfo{}, nil
	}
	if err != nil {
		return model.TokenInfo{}, err
	}
	info.HasToken = true
	return info, nil
}

// GetUserByTokenHash resolves an active account from a token hash. Returns
// (nil, nil) when no token matches or the owning account is inactive/missing, so
// a revoked or deactivated account's token authenticates no one. last_used_at is
// not touched here — call TouchUserToken for that.
func (s *Store) GetUserByTokenHash(tokenHash string) (*model.User, error) {
	row := s.db.QueryRow(
		`SELECT u.id, u.username, u.is_admin, u.is_active, u.permissions, u.created_at, COALESCE(u.last_login_at, ''), COALESCE(u.password_epoch, 0)
		   FROM user_tokens t JOIN users u ON u.id = t.user_id
		  WHERE t.token_hash = ?`, tokenHash)
	u, err := s.scanUser(row)
	if err != nil || u == nil || !u.IsActive {
		return nil, err
	}
	return u, nil
}

// TouchUserToken stamps a token's last_used_at with the current time, throttled
// so a busy client doesn't write on every request: it only updates when the
// stored value is null or at least a minute old. Best-effort — callers ignore
// the error (a failed stamp must never block an otherwise-valid request).
func (s *Store) TouchUserToken(tokenHash string) error {
	_, err := s.db.Exec(
		`UPDATE user_tokens SET last_used_at = CURRENT_TIMESTAMP
		  WHERE token_hash = ?
		    AND (last_used_at IS NULL OR last_used_at <= datetime('now', '-60 seconds'))`,
		tokenHash)
	return err
}

// DeleteUserToken revokes a user's token. Returns true if a token was removed.
func (s *Store) DeleteUserToken(userID int64) (bool, error) {
	res, err := s.db.Exec("DELETE FROM user_tokens WHERE user_id = ?", userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
