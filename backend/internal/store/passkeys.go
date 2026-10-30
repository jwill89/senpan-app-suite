package store

import (
	"context"
	"database/sql"
	"errors"

	"app-suite/internal/model"
)

// The passkey store deals only in strings: the server layer serializes the
// go-webauthn Credential to JSON (`credentialJSON`) and derives `credentialID`
// (base64url of the credential's raw ID), so the store stays free of any WebAuthn
// dependency.

// CreatePasskey inserts a stored WebAuthn credential for a user and returns its id.
func (s *Store) CreatePasskey(ctx context.Context, userID int64, credentialID, credentialJSON, name string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO user_passkeys (user_id, credential_id, credential, name) VALUES (?, ?, ?, ?)`,
		userID, credentialID, credentialJSON, name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListPasskeys returns a user's passkeys' public metadata (no key material).
func (s *Store) ListPasskeys(ctx context.Context, userID int64) ([]model.Passkey, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, created_at, last_used_at FROM user_passkeys WHERE user_id = ? ORDER BY created_at ASC`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.Passkey, 0)
	for rows.Next() {
		var p model.Passkey
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt, &p.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPasskeyCredentialsJSON returns the serialized credentials for a user (used to
// build the WebAuthn user for registration exclusions and login verification).
func (s *Store) GetPasskeyCredentialsJSON(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT credential FROM user_passkeys WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var j string
		if err := rows.Scan(&j); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// PasskeyCredentialExists reports whether a credential id is already registered
// (to anyone). Used to reject a duplicate registration.
func (s *Store) PasskeyCredentialExists(ctx context.Context, credentialID string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM user_passkeys WHERE credential_id = ?`, credentialID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// UpdatePasskeyCredential replaces the stored credential JSON (e.g. after a login
// bumps the signature counter) and stamps last_used_at.
func (s *Store) UpdatePasskeyCredential(ctx context.Context, credentialID, credentialJSON string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE user_passkeys SET credential = ?, last_used_at = datetime('now') WHERE credential_id = ?`,
		credentialJSON, credentialID)
	return err
}

// DeletePasskey removes one of a user's passkeys by row id (scoped to user_id so a
// user can't delete another account's). Returns true when a row was deleted.
func (s *Store) DeletePasskey(ctx context.Context, userID, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM user_passkeys WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
