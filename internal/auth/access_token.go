package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// HashAccessSecret hashes secret using argon2id with the prefix as salt.
// Same parameters as HashPassword for consistency.
//
// Using prefix as salt makes lookup-then-verify constant-cost per token (no
// guessing a random salt). The cryptographic value is in the secret's
// 32-byte entropy, not the salt's uniqueness.
func HashAccessSecret(secret, prefix string) (string, error) {
	if secret == "" || prefix == "" {
		return "", errors.New("auth: HashAccessSecret requires non-empty secret and prefix")
	}
	salt := []byte(prefix)
	key := argon2.IDKey([]byte(secret), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyAccessSecret checks secret+prefix against a stored PHC hash.
// Constant-time comparison.
func VerifyAccessSecret(secret, prefix, storedHash string) error {
	candidate, err := HashAccessSecret(secret, prefix)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(candidate), []byte(storedHash)) != 1 {
		return ErrIncorrectPassword
	}
	return nil
}
