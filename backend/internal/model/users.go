package model

// User is an admin-area account. Authentication is username + password only;
// the argon2id password hash is never part of this struct (it stays in the
// store layer) so it can never leak through JSON serialization.
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
	LastLoginAt string   `json:"last_login_at"` // ISO timestamp of last successful login ("" if never)
}

// TokenInfo is the non-secret metadata about an account's personal access token
// (used by external API clients such as the FFXIV plugin). The token plaintext
// is never stored or re-shown, so only its prefix and timestamps are exposed.
// HasToken is false (and the other fields empty) when the account has no token.
type TokenInfo struct {
	HasToken   bool   `json:"has_token"`
	Prefix     string `json:"prefix"`       // leading visible chars, e.g. "pat_Ab12Cd34"
	CreatedAt  string `json:"created_at"`   // ISO timestamp ("" when none)
	LastUsedAt string `json:"last_used_at"` // ISO timestamp ("" when never used)
}

// AuthCheckResponse is the body of GET /api/auth: whether a session is logged in
// and, if so, the active user. User is null when not authenticated, so it is NOT
// omitempty — the key is always present (null when logged out).
type AuthCheckResponse struct {
	Authenticated bool  `json:"authenticated"`
	User          *User `json:"user"`
}

// LoginResponse is the body of POST /api/auth {action:"login"} on success. The
// success key is always the literal true here, and the logged-in user is always
// present, so neither field is omitempty.
type LoginResponse struct {
	Success bool `json:"success"`
	User    User `json:"user"`
}

// LogoutResponse is the body of POST /api/auth {action:"logout"}: just the
// success flag. Kept separate from LoginResponse so logout never emits a
// user:null key (which would be a wire change from today's {"success":true}).
type LogoutResponse struct {
	Success bool `json:"success"`
}

// RegisterResponse is the body of POST /api/register: the success flag plus a
// human-readable message (the account is created pending admin activation).
type RegisterResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// UsersResponse is the body of GET /api/users — all accounts (admin only).
type UsersResponse struct {
	Users []User `json:"users"`
}

// AccountTokenGenerateResponse is the body of POST /api/account/token
// {action:"generate"}: the freshly minted token plaintext (returned EXACTLY
// once), its visible prefix, and the creation timestamp.
type AccountTokenGenerateResponse struct {
	Token     string `json:"token"`
	Prefix    string `json:"prefix"`
	CreatedAt string `json:"created_at"`
}

// TokenRevokeResponse is the body of POST /api/account/token {action:"revoke"}:
// the {"ok": true, "deleted": bool} shape reporting whether a token row was
// actually removed. (Distinct from the bare OKResponse — it carries deleted.)
type TokenRevokeResponse struct {
	OK      bool `json:"ok"`
	Deleted bool `json:"deleted"`
}
