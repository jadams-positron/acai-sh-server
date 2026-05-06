package specs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jadams-positron/acai-sh-server/internal/store"
	"github.com/jadams-positron/acai-sh-server/internal/store/sqlc"
)

// Repository wraps the sqlc queries for the specs domain.
type Repository struct{ db *store.DB }

// NewRepository returns a Repository over db.
func NewRepository(db *store.DB) *Repository { return &Repository{db: db} }

// ErrNotFound is returned when no matching row exists.
var ErrNotFound = errors.New("specs: not found")

// IsNotFound reports whether err is or wraps ErrNotFound.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// GetSpec returns the spec for (branchID, featureName) or ErrNotFound.
func (r *Repository) GetSpec(ctx context.Context, branchID, featureName string) (*Spec, error) {
	q := sqlc.New(r.db.Read)
	row, err := q.GetSpecByBranchAndFeature(ctx, sqlc.GetSpecByBranchAndFeatureParams{
		BranchID:    branchID,
		FeatureName: featureName,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("specs: GetSpec: %w", err)
	}
	return specFromRow(row)
}

// GetRefs returns the feature_branch_refs row for (branchID, featureName) or ErrNotFound.
func (r *Repository) GetRefs(ctx context.Context, branchID, featureName string) (*FeatureBranchRef, error) {
	q := sqlc.New(r.db.Read)
	row, err := q.GetFeatureBranchRef(ctx, sqlc.GetFeatureBranchRefParams{
		BranchID:    branchID,
		FeatureName: featureName,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("specs: GetRefs: %w", err)
	}
	return refsFromRow(row)
}

// GetStates returns the feature_impl_states row for (implID, featureName) or ErrNotFound.
func (r *Repository) GetStates(ctx context.Context, implID, featureName string) (*FeatureImplState, error) {
	q := sqlc.New(r.db.Read)
	row, err := q.GetFeatureImplState(ctx, sqlc.GetFeatureImplStateParams{
		ImplementationID: implID,
		FeatureName:      featureName,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("specs: GetStates: %w", err)
	}
	return statesFromRow(row)
}

// PickRefsBranch returns the branch from the impl's tracked branches that has
// the most-recent feature_branch_refs.pushed_at for featureName. ErrNotFound
// when no branch tracks any refs for that feature.
func (r *Repository) PickRefsBranch(ctx context.Context, implID, featureName string) (*Branch, error) {
	q := sqlc.New(r.db.Read)
	row, err := q.PickRefsBranchForFeature(ctx, sqlc.PickRefsBranchForFeatureParams{
		FeatureName:      featureName,
		ImplementationID: implID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("specs: PickRefsBranch: %w", err)
	}
	return branchFromRow(row), nil
}

// FirstTrackedBranch returns the first branch tracked by the impl (sorted by
// branches.updated_at DESC). ErrNotFound when the impl has no tracked branches.
func (r *Repository) FirstTrackedBranch(ctx context.Context, implID string) (*Branch, error) {
	q := sqlc.New(r.db.Read)
	rows, err := q.ListBranchesForImplementation(ctx, implID)
	if err != nil {
		return nil, fmt.Errorf("specs: ListBranchesForImplementation: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return branchFromRow(rows[0]), nil
}

// ListSpecsForBranch returns all specs on the given branch ordered by feature_name.
func (r *Repository) ListSpecsForBranch(ctx context.Context, branchID string) ([]*Spec, error) {
	q := sqlc.New(r.db.Read)
	rows, err := q.ListSpecsForBranch(ctx, branchID)
	if err != nil {
		return nil, fmt.Errorf("specs: ListSpecsForBranch: %w", err)
	}
	out := make([]*Spec, 0, len(rows))
	for i := range rows {
		s, err := specFromRow(rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// ListStatesForImpl returns all feature_impl_states rows for the impl.
func (r *Repository) ListStatesForImpl(ctx context.Context, implID string) ([]*FeatureImplState, error) {
	q := sqlc.New(r.db.Read)
	rows, err := q.ListFeatureImplStatesForImpl(ctx, implID)
	if err != nil {
		return nil, fmt.Errorf("specs: ListStatesForImpl: %w", err)
	}
	out := make([]*FeatureImplState, 0, len(rows))
	for i := range rows {
		s, err := statesFromRow(rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// ListRefsForBranch returns all feature_branch_refs rows for the branch.
func (r *Repository) ListRefsForBranch(ctx context.Context, branchID string) ([]*FeatureBranchRef, error) {
	q := sqlc.New(r.db.Read)
	rows, err := q.ListFeatureBranchRefsForBranch(ctx, branchID)
	if err != nil {
		return nil, fmt.Errorf("specs: ListRefsForBranch: %w", err)
	}
	out := make([]*FeatureBranchRef, 0, len(rows))
	for i := range rows {
		rf, err := refsFromRow(rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, rf)
	}
	return out, nil
}

func branchFromRow(row sqlc.Branch) *Branch {
	updatedAt, _ := time.Parse(time.RFC3339Nano, row.UpdatedAt)
	return &Branch{
		ID:             row.ID,
		TeamID:         row.TeamID,
		RepoURI:        row.RepoUri,
		BranchName:     row.BranchName,
		LastSeenCommit: row.LastSeenCommit,
		UpdatedAt:      updatedAt,
	}
}

func specFromRow(row sqlc.Spec) (*Spec, error) {
	insertedAt, _ := time.Parse(time.RFC3339Nano, row.InsertedAt)
	updatedAt, _ := time.Parse(time.RFC3339Nano, row.UpdatedAt)
	parsedAt, _ := time.Parse(time.RFC3339Nano, row.ParsedAt)

	var rawReqs map[string]struct {
		Requirement string   `json:"requirement"`
		Deprecated  *bool    `json:"deprecated,omitempty"`
		Note        *string  `json:"note,omitempty"`
		ReplacedBy  []string `json:"replaced_by,omitempty"`
	}
	if row.Requirements != "" {
		_ = json.Unmarshal([]byte(row.Requirements), &rawReqs)
	}
	reqs := make(map[string]Requirement, len(rawReqs))
	for k, v := range rawReqs {
		dep := false
		if v.Deprecated != nil {
			dep = *v.Deprecated
		}
		reqs[k] = Requirement{
			Requirement: v.Requirement,
			Deprecated:  dep,
			Note:        v.Note,
			ReplacedBy:  v.ReplacedBy,
		}
	}

	return &Spec{
		ID:                 row.ID,
		ProductID:          row.ProductID,
		BranchID:           row.BranchID,
		Path:               row.Path,
		LastSeenCommit:     row.LastSeenCommit,
		ParsedAt:           parsedAt,
		FeatureName:        row.FeatureName,
		FeatureDescription: row.FeatureDescription,
		FeatureVersion:     row.FeatureVersion,
		RawContent:         row.RawContent,
		Requirements:       reqs,
		InsertedAt:         insertedAt,
		UpdatedAt:          updatedAt,
	}, nil
}

func refsFromRow(row sqlc.FeatureBranchRef) (*FeatureBranchRef, error) {
	pushedAt, _ := time.Parse(time.RFC3339Nano, row.PushedAt)
	var raw map[string][]struct {
		Path   string `json:"path"`
		IsTest bool   `json:"is_test"`
	}
	if row.Refs != "" {
		_ = json.Unmarshal([]byte(row.Refs), &raw)
	}
	refs := make(map[string][]CodeRef, len(raw))
	for k, entries := range raw {
		codeRefs := make([]CodeRef, 0, len(entries))
		for _, e := range entries {
			codeRefs = append(codeRefs, CodeRef{Path: e.Path, IsTest: e.IsTest})
		}
		refs[k] = codeRefs
	}
	return &FeatureBranchRef{
		ID:          row.ID,
		BranchID:    row.BranchID,
		FeatureName: row.FeatureName,
		Refs:        refs,
		Commit:      row.Commit,
		PushedAt:    pushedAt,
	}, nil
}

func statesFromRow(row sqlc.FeatureImplState) (*FeatureImplState, error) {
	updatedAt, _ := time.Parse(time.RFC3339Nano, row.UpdatedAt)
	var raw map[string]struct {
		Status    *string `json:"status"`
		Comment   *string `json:"comment,omitempty"`
		UpdatedAt *string `json:"updated_at,omitempty"`
	}
	if row.States != "" {
		_ = json.Unmarshal([]byte(row.States), &raw)
	}
	states := make(map[string]ACIDState, len(raw))
	for k, v := range raw {
		st := ACIDState{Status: v.Status, Comment: v.Comment}
		if v.UpdatedAt != nil {
			if parsed, err := time.Parse(time.RFC3339Nano, *v.UpdatedAt); err == nil {
				st.UpdatedAt = &parsed
			}
		}
		states[k] = st
	}

	return &FeatureImplState{
		ID:               row.ID,
		ImplementationID: row.ImplementationID,
		FeatureName:      row.FeatureName,
		States:           states,
		UpdatedAt:        updatedAt,
	}, nil
}
