// Package products is the domain context for product CRUD.
package products

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jadams-positron/acai-sh-server/internal/store"
	"github.com/jadams-positron/acai-sh-server/internal/store/sqlc"
)

// Product mirrors a row in the products table.
type Product struct {
	ID          string
	TeamID      string
	Name        string
	Description *string
	IsActive    bool
	InsertedAt  time.Time
	UpdatedAt   time.Time
}

// Repository owns product reads.
type Repository struct {
	db *store.DB
}

// NewRepository returns a Repository over db.
func NewRepository(db *store.DB) *Repository { return &Repository{db: db} }

// ErrNotFound is returned when no matching product exists.
var ErrNotFound = errors.New("products: not found")

// IsNotFound reports whether err is or wraps ErrNotFound.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// GetByTeamAndName returns the active product matching (teamID, name) case-insensitively.
func (r *Repository) GetByTeamAndName(ctx context.Context, teamID, name string) (*Product, error) {
	q := sqlc.New(r.db.Read)
	row, err := q.GetProductByTeamAndName(ctx, sqlc.GetProductByTeamAndNameParams{
		TeamID: teamID,
		Name:   name,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("products: GetByTeamAndName: %w", err)
	}
	insertedAt, _ := time.Parse(time.RFC3339Nano, row.InsertedAt)
	updatedAt, _ := time.Parse(time.RFC3339Nano, row.UpdatedAt)
	return &Product{
		ID:          row.ID,
		TeamID:      row.TeamID,
		Name:        row.Name,
		Description: row.Description,
		IsActive:    row.IsActive != 0,
		InsertedAt:  insertedAt,
		UpdatedAt:   updatedAt,
	}, nil
}
