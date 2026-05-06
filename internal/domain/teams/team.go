// Package teams owns team membership, access tokens, and team-scoped lookups.
package teams

import "time"

// Team mirrors the teams table.
type Team struct {
	ID          string
	Name        string
	GlobalAdmin bool
	InsertedAt  time.Time
	UpdatedAt   time.Time
}
