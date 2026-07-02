package model

// Passkey is the public metadata for a stored WebAuthn credential. The key
// material (public key, signature counter, etc.) stays in the store and is never
// exposed through the API.
type Passkey struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at"`
}

// PasskeysResponse is the body of GET /api/account/passkeys.
type PasskeysResponse struct {
	Passkeys []Passkey `json:"passkeys"`
}
