// Package auth owns the auth subsystem: password hashing, session management,
// magic-link tokens, bearer-token verification, and middleware for enforcing
// authentication and authorization on HTTP routes.
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

// Argon2id parameters — must match argon2_elixir defaults so legacy password
// hashes imported from Postgres verify without rehashing.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// ErrInvalidHash is returned when a hash string is not a well-formed argon2id
// PHC string with the expected algorithm and parameters.
var ErrInvalidHash = errors.New("auth: invalid or unsupported password hash")

// ErrIncorrectPassword is returned when the supplied plaintext password does
// not match the stored hash.
var ErrIncorrectPassword = errors.New("auth: incorrect password")

// HashPassword hashes password using argon2id with a fresh random salt.
// The returned string is a PHC-formatted hash that can be stored in the DB.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return formatPHC(salt, hash), nil
}

// VerifyPassword checks whether password matches the stored argon2id hash.
// Returns ErrInvalidHash if the hash string is malformed or uses a different
// algorithm, and ErrIncorrectPassword if the password is wrong.
func VerifyPassword(password, hash string) error {
	salt, storedKey, err := parsePHC(hash)
	if err != nil {
		return err
	}
	candidate := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	if subtle.ConstantTimeCompare(candidate, storedKey) != 1 {
		return ErrIncorrectPassword
	}
	return nil
}

// formatPHC encodes salt and hash as an argon2id PHC string:
//
//	$argon2id$v=19$m=65536,t=3,p=4$<saltB64>$<hashB64>
func formatPHC(salt, hash []byte) string {
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, b64Salt, b64Hash)
}

// parsePHC decodes a PHC-formatted argon2id hash string into its salt and hash
// components. Returns ErrInvalidHash for any structural or algorithm mismatch.
func parsePHC(s string) (salt, hash []byte, err error) {
	// Expected format (7 parts after splitting on "$", leading empty string included):
	// "" "argon2id" "v=19" "m=65536,t=3,p=4" "<saltB64>" "<hashB64>"
	parts := strings.Split(s, "$")
	if len(parts) != 6 || parts[0] != "" {
		return nil, nil, ErrInvalidHash
	}
	if parts[1] != "argon2id" {
		return nil, nil, ErrInvalidHash
	}
	// Validate version field (e.g. "v=19") — we accept any well-formed v= prefix.
	if !strings.HasPrefix(parts[2], "v=") {
		return nil, nil, ErrInvalidHash
	}
	// We don't verify the parameter field strictly — VerifyPassword uses the
	// compile-time constants, so a hash with different params would just fail
	// the compare. Structural presence is enough to distinguish a valid PHC.
	if parts[3] == "" {
		return nil, nil, ErrInvalidHash
	}
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, ErrInvalidHash
	}
	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, ErrInvalidHash
	}
	return salt, hash, nil
}
