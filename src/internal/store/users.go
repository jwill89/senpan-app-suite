package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"app-suite/internal/model"
)

// ── User operations ──────────────────────────────────────────────────────────
//
// Permissions are stored as a JSON array string in the users.permissions column
// (the same pattern used by presets.pattern_ids and announcements.buttons). The
// argon2id password hash lives only here in the store layer and is never placed
// on model.User, so it cannot leak through API JSON.

// ErrUsernameTaken is returned by CreateUser when the username already exists.
var ErrUsernameTaken = errors.New("username already taken")

// CreateUser inserts a new account. New accounts are inactive and non-admin
// with no page permissions; an admin must activate and grant access. Returns
// ErrUsernameTaken if the username is already in use.
func (s *Store) CreateUser(username, passwordHash string) (*model.User, error) {
	res, err := s.db.Exec(
		`INSERT INTO users (username, password_hash, is_admin, is_active, permissions)
		 VALUES (?, ?, 0, 0, '[]')`,
		username, passwordHash,
	)
	if err != nil {
		// The pure-Go sqlite driver surfaces a UNIQUE violation in the error text.
		if isUniqueViolation(err) {
			return nil, ErrUsernameTaken
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetUserByID(id)
}

// GetUserByID retrieves a user by id. Returns nil if not found.
func (s *Store) GetUserByID(id int64) (*model.User, error) {
	return s.scanUser(s.db.QueryRow(
		`SELECT id, username, is_admin, is_active, permissions, created_at
		 FROM users WHERE id = ?`, id))
}

// GetUserByUsername retrieves a user plus its password hash, used by login.
// Returns (nil, "", nil) if no such user exists.
func (s *Store) GetUserByUsername(username string) (*model.User, string, error) {
	var u model.User
	var permsJSON, hash string
	var isAdmin, isActive int
	err := s.db.QueryRow(
		`SELECT id, username, password_hash, is_admin, is_active, permissions, created_at
		 FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &hash, &isAdmin, &isActive, &permsJSON, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	u.IsAdmin = isAdmin == 1
	u.IsActive = isActive == 1
	if err := json.Unmarshal([]byte(permsJSON), &u.Permissions); err != nil {
		return nil, "", fmt.Errorf("unmarshal permissions: %w", err)
	}
	return &u, hash, nil
}

// ListUsers returns all users ordered by username (no password hashes).
func (s *Store) ListUsers() ([]model.User, error) {
	rows, err := s.db.Query(
		`SELECT id, username, is_admin, is_active, permissions, created_at
		 FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]model.User, 0)
	for rows.Next() {
		var u model.User
		var permsJSON string
		var isAdmin, isActive int
		if err := rows.Scan(&u.ID, &u.Username, &isAdmin, &isActive, &permsJSON, &u.CreatedAt); err != nil {
			return nil, err
		}
		u.IsAdmin = isAdmin == 1
		u.IsActive = isActive == 1
		if err := json.Unmarshal([]byte(permsJSON), &u.Permissions); err != nil {
			return nil, fmt.Errorf("unmarshal permissions: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// UserExists reports whether an account with the given username exists.
func (s *Store) UserExists(username string) (bool, error) {
	var n int
	err := s.db.QueryRow("SELECT 1 FROM users WHERE username = ?", username).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// CountUsers returns the total number of accounts.
func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&n)
	return n, err
}

// SetUserActive activates or deactivates an account.
func (s *Store) SetUserActive(id int64, active bool) error {
	_, err := s.db.Exec("UPDATE users SET is_active = ? WHERE id = ?", boolToInt(active), id)
	return err
}

// SetUserAdmin grants or revokes admin (full-access) status.
func (s *Store) SetUserAdmin(id int64, admin bool) error {
	_, err := s.db.Exec("UPDATE users SET is_admin = ? WHERE id = ?", boolToInt(admin), id)
	return err
}

// SetUserPermissions replaces a user's page-permission set.
func (s *Store) SetUserPermissions(id int64, perms []string) error {
	if perms == nil {
		perms = []string{}
	}
	data, err := json.Marshal(perms)
	if err != nil {
		return fmt.Errorf("marshal permissions: %w", err)
	}
	_, err = s.db.Exec("UPDATE users SET permissions = ? WHERE id = ?", string(data), id)
	return err
}

// SetUserPassword updates a user's argon2id password hash.
func (s *Store) SetUserPassword(id int64, passwordHash string) error {
	_, err := s.db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", passwordHash, id)
	return err
}

// DeleteUser removes an account. Returns true if a row was deleted.
func (s *Store) DeleteUser(id int64) (bool, error) {
	res, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// scanUser scans a single user row (without the password hash). Returns nil
// if the row does not exist.
func (s *Store) scanUser(row *sql.Row) (*model.User, error) {
	var u model.User
	var permsJSON string
	var isAdmin, isActive int
	err := row.Scan(&u.ID, &u.Username, &isAdmin, &isActive, &permsJSON, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.IsAdmin = isAdmin == 1
	u.IsActive = isActive == 1
	if err := json.Unmarshal([]byte(permsJSON), &u.Permissions); err != nil {
		return nil, fmt.Errorf("unmarshal permissions: %w", err)
	}
	return &u, nil
}

// isUniqueViolation reports whether err is a SQLite UNIQUE-constraint failure
// (the pure-Go driver surfaces it in the error text).
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
