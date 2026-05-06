// Package products is the domain context for product CRUD.
package products

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

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

// ErrInvalidProductName is returned for malformed product names.
var ErrInvalidProductName = errors.New("products: invalid product name")

// IsInvalidProductName reports whether err is or wraps ErrInvalidProductName.
func IsInvalidProductName(err error) bool { return errors.Is(err, ErrInvalidProductName) }

// ErrDuplicateName is returned for UNIQUE constraint conflicts on (team_id, name).
var ErrDuplicateName = errors.New("products: duplicate product name")

// IsDuplicateName reports whether err is or wraps ErrDuplicateName.
func IsDuplicateName(err error) bool { return errors.Is(err, ErrDuplicateName) }

// GetOrCreate returns the team-scoped product by name, creating it if absent.
// Idempotent under concurrent calls — uses INSERT and falls back to SELECT
// on UNIQUE conflict (team_id, name).
func (r *Repository) GetOrCreate(ctx context.Context, teamID, name string) (*Product, error) {
	if existing, err := r.GetByTeamAndName(ctx, teamID, name); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("products: gen uuid: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	q := sqlc.New(r.db.Write)
	row, err := q.CreateProduct(ctx, sqlc.CreateProductParams{
		ID:         id.String(),
		TeamID:     teamID,
		Name:       name,
		InsertedAt: now,
		UpdatedAt:  now,
	})
	if err != nil {
		// Race condition: another caller may have inserted concurrently.
		// Re-fetch by name and return that.
		if existing, err2 := r.GetByTeamAndName(ctx, teamID, name); err2 == nil {
			return existing, nil
		}
		return nil, fmt.Errorf("products: CreateProduct: %w", err)
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

// ListForTeam returns all active products under teamID ordered by name.
func (r *Repository) ListForTeam(ctx context.Context, teamID string) ([]*Product, error) {
	q := sqlc.New(r.db.Read)
	rows, err := q.ListProductsForTeam(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("products: ListForTeam: %w", err)
	}
	out := make([]*Product, 0, len(rows))
	for _, row := range rows {
		insertedAt, _ := time.Parse(time.RFC3339Nano, row.InsertedAt)
		updatedAt, _ := time.Parse(time.RFC3339Nano, row.UpdatedAt)
		out = append(out, &Product{
			ID:          row.ID,
			TeamID:      row.TeamID,
			Name:        row.Name,
			Description: row.Description,
			IsActive:    row.IsActive != 0,
			InsertedAt:  insertedAt,
			UpdatedAt:   updatedAt,
		})
	}
	return out, nil
}

// validateProductName enforces alphanumeric + hyphens/underscores/periods, 1-64 chars.
func validateProductName(name string) error {
	if len(name) < 1 || len(name) > 64 {
		return fmt.Errorf("%w: length %d", ErrInvalidProductName, len(name))
	}
	for _, c := range name {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.'
		if !ok {
			return fmt.Errorf("%w: invalid character %q", ErrInvalidProductName, c)
		}
	}
	return nil
}

// Create inserts a new product. Returns ErrInvalidProductName for bad names,
// ErrDuplicateName for UNIQUE conflicts.
func (r *Repository) Create(ctx context.Context, teamID, name string) (*Product, error) {
	if err := validateProductName(name); err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("products: gen uuid: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	q := sqlc.New(r.db.Write)
	row, err := q.CreateProduct(ctx, sqlc.CreateProductParams{
		ID:         id.String(),
		TeamID:     teamID,
		Name:       name,
		InsertedAt: now,
		UpdatedAt:  now,
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateName, name)
		}
		return nil, fmt.Errorf("products: Create: %w", err)
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
