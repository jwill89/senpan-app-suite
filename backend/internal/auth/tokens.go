package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// A personal access token lets a client that isn't a browser (the plugin) act as
// its account, sent as `Authorization: Bearer <token>`. It is "pat_" and 32 random
// bytes in base64url: 256 bits, so a fast SHA-256 is the right hash to keep (there
// is nothing low-entropy to guess), and looking one up stays a single indexed read.
// Only the hash is stored; the plaintext is shown once, when it is made.
const (
	TokenScheme = "pat_"
	// tokenPrefixLength is how much of a token the account page shows to tell it by.
	tokenPrefixLength = 12
	tokenBytes        = 32
)

// NewToken makes a token: its plaintext (shown once), the hash to keep, and the
// prefix the account page shows.
func NewToken() (token, hash, prefix string, err error) {
	buf := make([]byte, tokenBytes)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", err
	}
	token = TokenScheme + base64.RawURLEncoding.EncodeToString(buf)
	return token, HashToken(token), token[:tokenPrefixLength], nil
}

// HashToken is the hex SHA-256 a token is kept and looked up by.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// linkTokenBytes is the size of the secret in an admin's Discord link.
const linkTokenBytes = 32

// NewLinkToken makes the secret of a link an admin hands out (to sign up, or to link a
// Discord account): 32 random bytes in base64url, and the hash it is kept and looked
// up by (HashToken).
func NewLinkToken() (token, hash string, err error) {
	buf := make([]byte, linkTokenBytes)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(buf)
	return token, HashToken(token), nil
}
