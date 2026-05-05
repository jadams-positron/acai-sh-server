package auth_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/acai-sh/server/internal/auth"
)

func TestHash_RoundTrip(t *testing.T) {
	hash, err := auth.HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if err := auth.VerifyPassword("correct-horse-battery-staple", hash); err != nil {
		t.Errorf("VerifyPassword() round-trip failed: %v", err)
	}
}

func TestHash_DifferentSaltsForSamePassword(t *testing.T) {
	h1, err := auth.HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword() first call error = %v", err)
	}
	h2, err := auth.HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword() second call error = %v", err)
	}
	if h1 == h2 {
		t.Errorf("HashPassword() returned identical hashes for the same password; salts must differ")
	}
}

func TestVerify_RejectsMalformedHash(t *testing.T) {
	err := auth.VerifyPassword("password", "not-a-valid-phc-string")
	if err == nil {
		t.Fatalf("VerifyPassword() with malformed hash should have errored, got nil")
	}
	if !errors.Is(err, auth.ErrInvalidHash) {
		t.Errorf("VerifyPassword() error = %v, want auth.ErrInvalidHash", err)
	}
}

func TestVerify_RejectsWrongAlgorithm(t *testing.T) {
	// Craft a PHC string that looks structurally valid but uses argon2i, not argon2id.
	wrongAlgo := "$argon2i$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$" + strings.Repeat("A", 43)
	err := auth.VerifyPassword("password", wrongAlgo)
	if err == nil {
		t.Fatalf("VerifyPassword() with argon2i hash should have errored, got nil")
	}
	if !errors.Is(err, auth.ErrInvalidHash) {
		t.Errorf("VerifyPassword() error = %v, want auth.ErrInvalidHash", err)
	}
}
