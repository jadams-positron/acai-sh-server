package implementations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/jadams-positron/acai-sh-server/internal/store"
	"github.com/jadams-positron/acai-sh-server/internal/store/sqlc"
)

// Repository owns implementation listings.
type Repository struct {
	db *store.DB
}

// NewRepository returns a Repository over db.
func NewRepository(db *store.DB) *Repository { return &Repository{db: db} }

// ListByTeamParams optionally narrows the listing.
type ListByTeamParams struct {
	TeamID     string
	ProductID  *string // optional
	RepoURI    *string // requires BranchName
	BranchName *string // requires RepoURI
}

// ErrNotFound is returned when no matching implementation exists.
var ErrNotFound = errors.New("implementations: not found")

// IsNotFound reports whether err is or wraps ErrNotFound.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// ErrInvalidParams is returned for nonsensical filter combinations.
var ErrInvalidParams = errors.New("implementations: invalid params")

// IsInvalidParams reports whether err is ErrInvalidParams.
func IsInvalidParams(err error) bool { return errors.Is(err, ErrInvalidParams) }

// List returns implementations for the team filtered by the given params.
// Branch filtering requires both RepoURI AND BranchName (or neither).
func (r *Repository) List(ctx context.Context, p ListByTeamParams) ([]*Implementation, error) {
	if (p.RepoURI != nil) != (p.BranchName != nil) {
		return nil, fmt.Errorf("%w: repo_uri and branch_name must be used together", ErrInvalidParams)
	}

	q := sqlc.New(r.db.Read)
	var impls []*Implementation

	switch {
	case p.ProductID == nil && p.RepoURI == nil:
		rows, err := q.ListImplementationsByTeam(ctx, p.TeamID)
		if err != nil {
			return nil, fmt.Errorf("implementations: list-by-team: %w", err)
		}
		for i := range rows {
			r := &rows[i]
			impls = append(impls, fromRow(r.ID, r.ProductID, r.TeamID, r.ParentImplementationID,
				r.Name, r.Description, r.IsActive, r.InsertedAt, r.UpdatedAt, r.ProductName))
		}

	case p.ProductID != nil && p.RepoURI == nil:
		rows, err := q.ListImplementationsByProduct(ctx, sqlc.ListImplementationsByProductParams{
			TeamID:    p.TeamID,
			ProductID: *p.ProductID,
		})
		if err != nil {
			return nil, fmt.Errorf("implementations: list-by-product: %w", err)
		}
		for i := range rows {
			r := &rows[i]
			impls = append(impls, fromRow(r.ID, r.ProductID, r.TeamID, r.ParentImplementationID,
				r.Name, r.Description, r.IsActive, r.InsertedAt, r.UpdatedAt, r.ProductName))
		}

	case p.ProductID == nil && p.RepoURI != nil:
		rows, err := q.ListImplementationsByBranch(ctx, sqlc.ListImplementationsByBranchParams{
			TeamID:     p.TeamID,
			RepoUri:    *p.RepoURI,
			BranchName: *p.BranchName,
		})
		if err != nil {
			return nil, fmt.Errorf("implementations: list-by-branch: %w", err)
		}
		for i := range rows {
			r := &rows[i]
			impls = append(impls, fromRow(r.ID, r.ProductID, r.TeamID, r.ParentImplementationID,
				r.Name, r.Description, r.IsActive, r.InsertedAt, r.UpdatedAt, r.ProductName))
		}

	default: // ProductID != nil && RepoURI != nil
		rows, err := q.ListImplementationsByProductAndBranch(ctx, sqlc.ListImplementationsByProductAndBranchParams{
			TeamID:     p.TeamID,
			ProductID:  *p.ProductID,
			RepoUri:    *p.RepoURI,
			BranchName: *p.BranchName,
		})
		if err != nil {
			return nil, fmt.Errorf("implementations: list-by-product-and-branch: %w", err)
		}
		for i := range rows {
			r := &rows[i]
			impls = append(impls, fromRow(r.ID, r.ProductID, r.TeamID, r.ParentImplementationID,
				r.Name, r.Description, r.IsActive, r.InsertedAt, r.UpdatedAt, r.ProductName))
		}
	}

	return impls, nil
}

// CreateImplementationParams holds the inputs for Create.
type CreateImplementationParams struct {
	ProductID              string
	TeamID                 string
	Name                   string
	ParentImplementationID *string
}

// Create inserts a new implementation row and returns it.
// The caller is responsible for ensuring uniqueness (no duplicate name check here).
func (r *Repository) Create(ctx context.Context, p CreateImplementationParams) (*Implementation, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("implementations: gen uuid: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	q := sqlc.New(r.db.Write)
	row, err := q.CreateImplementation(ctx, sqlc.CreateImplementationParams{
		ID:                     id.String(),
		ProductID:              p.ProductID,
		TeamID:                 p.TeamID,
		ParentImplementationID: p.ParentImplementationID,
		Name:                   p.Name,
		InsertedAt:             now,
		UpdatedAt:              now,
	})
	if err != nil {
		return nil, fmt.Errorf("implementations: Create: %w", err)
	}
	return fromRow(row.ID, row.ProductID, row.TeamID, row.ParentImplementationID,
		row.Name, row.Description, row.IsActive,
		row.InsertedAt, row.UpdatedAt, ""), nil
}

// ListTrackingBranch returns the active implementations under team that track
// the given branch (by branch ID). Used by /push to infer the target impl
// when the caller doesn't pass target_impl_name.
func (r *Repository) ListTrackingBranch(ctx context.Context, teamID, branchID string) ([]*Implementation, error) {
	q := sqlc.New(r.db.Read)
	rows, err := q.ListImplementationsTrackingBranch(ctx, sqlc.ListImplementationsTrackingBranchParams{
		TeamID:   teamID,
		BranchID: branchID,
	})
	if err != nil {
		return nil, fmt.Errorf("implementations: ListTrackingBranch: %w", err)
	}
	out := make([]*Implementation, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		out = append(out, fromRow(row.ID, row.ProductID, row.TeamID, row.ParentImplementationID,
			row.Name, row.Description, row.IsActive, row.InsertedAt, row.UpdatedAt, row.ProductName))
	}
	return out, nil
}

// GetByProductAndName returns the active implementation under product with the
// given name, or ErrNotFound.
func (r *Repository) GetByProductAndName(ctx context.Context, productID, name string) (*Implementation, error) {
	q := sqlc.New(r.db.Read)
	row, err := q.GetImplementationByProductAndName(ctx, sqlc.GetImplementationByProductAndNameParams{
		ProductID: productID,
		Name:      name,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("implementations: GetByProductAndName: %w", err)
	}
	return fromRow(row.ID, row.ProductID, row.TeamID, row.ParentImplementationID,
		row.Name, row.Description, row.IsActive, row.InsertedAt, row.UpdatedAt, ""), nil
}

func fromRow(id, productID, teamID string, parentID *string, name string, description *string, isActive int64,
	insertedAtStr, updatedAtStr, productName string) *Implementation {
	insertedAt, _ := time.Parse(time.RFC3339Nano, insertedAtStr)
	updatedAt, _ := time.Parse(time.RFC3339Nano, updatedAtStr)
	return &Implementation{
		ID:                     id,
		ProductID:              productID,
		TeamID:                 teamID,
		ParentImplementationID: parentID,
		Name:                   name,
		Description:            description,
		IsActive:               isActive != 0,
		ProductName:            productName,
		InsertedAt:             insertedAt,
		UpdatedAt:              updatedAt,
	}
}
