package teams

import "time"

// AccessToken mirrors the access_tokens table (sans the hash, which is internal).
type AccessToken struct {
	ID          string
	UserID      string
	TeamID      string
	Name        string
	TokenPrefix string
	Scopes      []string
	ExpiresAt   *time.Time
	RevokedAt   *time.Time
	LastUsedAt  *time.Time
	InsertedAt  time.Time
	UpdatedAt   time.Time
}

// IsValid reports whether the token is not revoked and not expired (against now).
func (t *AccessToken) IsValid(now time.Time) bool {
	if t == nil {
		return false
	}
	if t.RevokedAt != nil {
		return false
	}
	if t.ExpiresAt != nil && !t.ExpiresAt.After(now) {
		return false
	}
	return true
}
