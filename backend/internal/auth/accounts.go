package auth

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"app-suite/internal/model"
)

// ReservedUsername is the bootstrap admin every new database is seeded with. Signing up
// can't take its name, and nobody deactivates, demotes or deletes it, or sets its
// password but the account itself.
const ReservedUsername = "admin"

// The limits on what an account is made of.
const (
	MinPasswordLength = 8
	MaxPasswordLength = 1024
	MaxUsernameLength = 32
	// MaxPasskeyName is the longest name a passkey keeps, in bytes.
	MaxPasskeyName = 60
)

// Row is an account as every account query reads it: db.GetUserRow and the rows of
// the other account queries have these fields in this order, so each converts to a
// Row. The password hash is never among them.
type Row struct {
	ID            int64
	Username      string
	IsAdmin       int64
	IsActive      int64
	Permissions   string
	CreatedAt     *string
	LastLoginAt   *string
	PasswordEpoch int64
}

// User is the account a row holds.
func (r Row) User() (*model.User, error) {
	u := &model.User{
		ID:            r.ID,
		Username:      r.Username,
		IsAdmin:       r.IsAdmin == 1,
		IsActive:      r.IsActive == 1,
		PasswordEpoch: r.PasswordEpoch,
	}
	if r.CreatedAt != nil {
		u.CreatedAt = *r.CreatedAt
	}
	if r.LastLoginAt != nil {
		u.LastLoginAt = *r.LastLoginAt
	}
	if err := json.Unmarshal([]byte(r.Permissions), &u.Permissions); err != nil {
		return nil, fmt.Errorf("read the permissions of account %d: %w", r.ID, err)
	}
	if u.Permissions == nil {
		u.Permissions = []string{}
	}
	return u, nil
}

// EncodePermissions is the stored form of a set of permission keys: a JSON array.
func EncodePermissions(keys []string) string {
	if keys == nil {
		keys = []string{}
	}
	b, _ := json.Marshal(keys) // a []string always encodes
	return string(b)
}

// PasskeyName is the name a passkey is kept under: trimmed, cut to MaxPasskeyName
// bytes on a character boundary, or "Passkey" when there is none.
func PasskeyName(raw string) string {
	name := strings.TrimSpace(raw)
	if len(name) > MaxPasskeyName {
		cut := MaxPasskeyName
		for cut > 0 && !utf8.RuneStart(name[cut]) {
			cut--
		}
		name = strings.TrimSpace(name[:cut])
	}
	if name == "" {
		return "Passkey"
	}
	return name
}
