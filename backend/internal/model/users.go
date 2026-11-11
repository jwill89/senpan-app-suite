package model

// User is an admin-area account, signed in with a password or a passkey; the
// argon2id password hash is never part of this struct (it stays in the store
// layer) so it can never leak through JSON serialization.
//
// Accounts are created inactive (via the hidden registration page) and remain
// so until an admin activates them. IsAdmin grants full access to every page;
// non-admin users are limited to the pages listed in Permissions (one key per
// admin page, mirroring the frontend AdminTab ids, e.g. "bingo-cards").
type User struct {
	ID          int64    `json:"id"`
	Username    string   `json:"username"`
	IsAdmin     bool     `json:"is_admin"`
	IsActive    bool     `json:"is_active"`
	Permissions []string `json:"permissions"` // page-permission keys (ignored when IsAdmin)
	CreatedAt   string   `json:"created_at"`
	LastLoginAt string   `json:"last_login_at"` // RFC 3339 time of the last successful login ("" if never)
	// PasswordEpoch increments on every password change/reset. A cookie session
	// stores the epoch it was minted with; currentUser rejects a session whose
	// epoch doesn't match, so changing a password logs out every other session.
	// Never serialized to clients.
	PasswordEpoch int64 `json:"-"`
}
