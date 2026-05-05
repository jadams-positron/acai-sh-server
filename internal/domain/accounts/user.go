// Package accounts is the domain context for user accounts and authentication.
// It owns the User and EmailToken value types, and the Repository that
// persists them to SQLite via the sqlc-generated query layer.
package accounts

import "time"

// User is the in-memory representation of an acai user.
type User struct {
	ID             string
	Email          string
	HashedPassword string
	ConfirmedAt    *time.Time
	InsertedAt     time.Time
	UpdatedAt      time.Time
}
