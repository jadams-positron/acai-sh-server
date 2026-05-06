// Package implementations is the domain context for implementation entities,
// branches, and the relationships between them.
package implementations

import "time"

// Implementation mirrors a row in implementations + the joined product name.
type Implementation struct {
	ID                     string
	ProductID              string
	TeamID                 string
	ParentImplementationID *string
	Name                   string
	Description            *string
	IsActive               bool
	ProductName            string // joined from products.name
	InsertedAt             time.Time
	UpdatedAt              time.Time
}
