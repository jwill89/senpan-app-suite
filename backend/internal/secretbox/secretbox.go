// Package secretbox seals short secrets for storage in the database - today the Discord
// bot token - with AES-256-GCM under a key that never lives in the database.
//
// The key is derived (HKDF-SHA256) from a server secret the process already holds: the
// session secret. A copy of the database (the live copy deploy.ps1 pulls to a dev machine,
// a backup) therefore carries only ciphertext, which is useless without that secret.
// Changing the session secret makes previously sealed values unreadable; callers say so and
// ask for the secret to be entered again.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

// version prefixes every sealed value, so the format can change without guessing.
const version = "v1:"

// ErrUnreadable is returned when a sealed value can't be opened: it was sealed under a
// different secret, or it is damaged, or it isn't a sealed value at all.
var ErrUnreadable = errors.New("secretbox: value can't be opened with this server's secret")

// Box seals and opens values under one derived key.
type Box struct {
	aead cipher.AEAD
}

// New derives a key from secret for one purpose (an HKDF info string, so the same secret
// yields unrelated keys for unrelated uses) and returns a Box using it.
func New(secret, purpose string) (*Box, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("secretbox: empty secret")
	}
	key, err := hkdf.Key(sha256.New, []byte(secret), nil, purpose, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext with a fresh random nonce and returns "v1:" + base64(nonce|ciphertext).
func (b *Box) Seal(plaintext string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := b.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return version + base64.RawStdEncoding.EncodeToString(sealed), nil
}

// Open reverses Seal. Anything it can't authenticate is ErrUnreadable.
func (b *Box) Open(sealed string) (string, error) {
	rest, ok := strings.CutPrefix(sealed, version)
	if !ok {
		return "", ErrUnreadable
	}
	raw, err := base64.RawStdEncoding.DecodeString(rest)
	if err != nil || len(raw) < b.aead.NonceSize() {
		return "", ErrUnreadable
	}
	nonce, ciphertext := raw[:b.aead.NonceSize()], raw[b.aead.NonceSize():]
	plain, err := b.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrUnreadable
	}
	return string(plain), nil
}
