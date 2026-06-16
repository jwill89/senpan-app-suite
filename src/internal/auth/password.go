// Package auth provides password hashing and verification using argon2id.
// It lives in its own package (depending only on golang.org/x/crypto) so both
// the store layer (seeding the bootstrap admin) and the server layer
// (login / register / change-password) can hash and verify without an import
// cycle.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters. Tuned for interactive admin logins on modest hardware:
// 64 MB memory with a single pass is the OWASP-recommended argon2id baseline.
const (
	argonTime    = 1
	argonMemory  = 64 * 1024 // KiB → 64 MB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// ErrInvalidHash is returned when an encoded hash is not a recognized
// argon2id PHC string.
var ErrInvalidHash = errors.New("invalid argon2id hash format")

// Hash derives an argon2id hash of password with a fresh random salt and
// returns it in PHC string format:
//
//	$argon2id$v=19$m=65536,t=1,p=4$<b64 salt>$<b64 hash>
func Hash(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// dummyHash is a real argon2id hash (of a throwaway password) computed once at
// startup. It backs DummyVerify so a login attempt for a non-existent username
// performs the same argon2 work as one for a real account.
var dummyHash, _ = Hash("argon2id-timing-equalization-placeholder")

// DummyVerify runs a verification against an internal fixed hash and discards
// the result. Call it on the no-such-user branch of a login so the response
// time doesn't reveal whether the username exists (defeats user enumeration via
// the otherwise-skipped, costly argon2 computation).
func DummyVerify(password string) {
	_, _ = Verify(password, dummyHash)
}

// Verify reports whether password matches the encoded argon2id PHC hash. It
// reads the parameters and salt from the encoded value, recomputes the hash,
// and compares in constant time. Returns ErrInvalidHash (or a decode error)
// when encoded is malformed.
func Verify(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=...,t=...,p=...", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, ErrInvalidHash
	}
	if version != argon2.Version {
		return false, ErrInvalidHash
	}

	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ErrInvalidHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, ErrInvalidHash
	}

	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
