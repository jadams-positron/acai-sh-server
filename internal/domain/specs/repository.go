package specs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/google/uuid"

	"github.com/jadams-positron/acai-sh-server/internal/store"
	"github.com/jadams-positron/acai-sh-server/internal/store/sqlc"
)

// UpsertSpecParams groups the inputs for UpsertSpec.
type UpsertSpecParams struct {
	ProductID          string
	BranchID           string
	Path               *string
	LastSeenCommit     string
	FeatureName        string
	FeatureDescription *string
	FeatureVersion     string
	RawContent         *string
	Requirements       map[string]Requirement
}

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

// ListBranchesForTeam returns all branches in the team, newest-updated first.
func (r *Repository) ListBranchesForTeam(ctx context.Context, teamID string) ([]*Branch, error) {
	q := sqlc.New(r.db.Read)
	rows, err := q.ListBranchesForTeam(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("specs: ListBranchesForTeam: %w", err)
	}
	out := make([]*Branch, 0, len(rows))
	for _, row := range rows {
		out = append(out, branchFromRow(row))
	}
	return out, nil
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

// UpsertStates writes the states JSON map for (implID, featureName), creating
// the row if missing. Caller is responsible for the merge (this is a full
// replace). If you need merge-with-existing, call GetStates first, mutate, pass
// the resulting map.
func (r *Repository) UpsertStates(ctx context.Context, implID, featureName string, states map[string]ACIDState) error {
	type acidStateJSON struct {
		Status    *string `json:"status"`
		Comment   *string `json:"comment,omitempty"`
		UpdatedAt *string `json:"updated_at,omitempty"`
	}
	out := make(map[string]acidStateJSON, len(states))
	for k, v := range states {
		entry := acidStateJSON{Status: v.Status, Comment: v.Comment}
		if v.UpdatedAt != nil {
			s := v.UpdatedAt.UTC().Format(time.RFC3339Nano)
			entry.UpdatedAt = &s
		}
		out[k] = entry
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("specs: marshal states: %w", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("specs: gen uuid: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	q := sqlc.New(r.db.Write)
	if err := q.UpsertFeatureImplState(ctx, sqlc.UpsertFeatureImplStateParams{
		ID:               id.String(),
		ImplementationID: implID,
		FeatureName:      featureName,
		States:           string(raw),
		InsertedAt:       now,
		UpdatedAt:        now,
	}); err != nil {
		return fmt.Errorf("specs: UpsertFeatureImplState: %w", err)
	}
	return nil
}

// UpsertBranch finds or creates the branch row for (teamID, repoURI, branchName).
// On found: updates last_seen_commit. On insert: full row.
// Returns (branch, created, error).
func (r *Repository) UpsertBranch(ctx context.Context, teamID, repoURI, branchName, commit string) (*Branch, bool, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	qr := sqlc.New(r.db.Read)
	qw := sqlc.New(r.db.Write)

	row, err := qr.GetBranchByTeamRepoAndName(ctx, sqlc.GetBranchByTeamRepoAndNameParams{
		TeamID:     teamID,
		RepoUri:    repoURI,
		BranchName: branchName,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, false, fmt.Errorf("specs: UpsertBranch get: %w", err)
	}

	if !errors.Is(err, sql.ErrNoRows) {
		// Found — update last_seen_commit.
		if err2 := qw.UpdateBranchLastSeenCommit(ctx, sqlc.UpdateBranchLastSeenCommitParams{
			LastSeenCommit: commit,
			UpdatedAt:      now,
			ID:             row.ID,
		}); err2 != nil {
			return nil, false, fmt.Errorf("specs: UpsertBranch update: %w", err2)
		}
		row.LastSeenCommit = commit
		row.UpdatedAt = now
		return branchFromRow(row), false, nil
	}

	// Not found — insert.
	id, err := uuid.NewV7()
	if err != nil {
		return nil, false, fmt.Errorf("specs: UpsertBranch uuid: %w", err)
	}
	newRow, err := qw.CreateBranch(ctx, sqlc.CreateBranchParams{
		ID:             id.String(),
		TeamID:         teamID,
		RepoUri:        repoURI,
		BranchName:     branchName,
		LastSeenCommit: commit,
		InsertedAt:     now,
		UpdatedAt:      now,
	})
	if err != nil {
		return nil, false, fmt.Errorf("specs: UpsertBranch insert: %w", err)
	}
	return branchFromRow(newRow), true, nil
}

// UpsertSpec finds or creates the spec for (productID, branchID, featureName).
// Returns (spec, created, error) — created is true when the row did not exist before.
func (r *Repository) UpsertSpec(ctx context.Context, p UpsertSpecParams) (*Spec, bool, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	qr := sqlc.New(r.db.Read)
	qw := sqlc.New(r.db.Write)

	// Check whether the row exists before we upsert (to decide created vs updated).
	_, getErr := qr.GetSpecByBranchAndFeature(ctx, sqlc.GetSpecByBranchAndFeatureParams{
		BranchID:    p.BranchID,
		FeatureName: p.FeatureName,
	})
	existed := !errors.Is(getErr, sql.ErrNoRows)
	if getErr != nil && !errors.Is(getErr, sql.ErrNoRows) {
		return nil, false, fmt.Errorf("specs: UpsertSpec get: %w", getErr)
	}

	// Marshal requirements using a local type with JSON tags.
	type reqJSON struct {
		Requirement string   `json:"requirement"`
		Deprecated  bool     `json:"deprecated,omitempty"`
		Note        *string  `json:"note,omitempty"`
		ReplacedBy  []string `json:"replaced_by,omitempty"`
	}
	reqOut := make(map[string]reqJSON, len(p.Requirements))
	for k, v := range p.Requirements {
		reqOut[k] = reqJSON(v)
	}
	reqsRaw, err := json.Marshal(reqOut)
	if err != nil {
		return nil, false, fmt.Errorf("specs: UpsertSpec marshal requirements: %w", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, false, fmt.Errorf("specs: UpsertSpec uuid: %w", err)
	}

	featureVersion := p.FeatureVersion
	if featureVersion == "" {
		featureVersion = "1.0.0"
	}

	row, err := qw.UpsertSpec(ctx, sqlc.UpsertSpecParams{
		ID:                 id.String(),
		ProductID:          p.ProductID,
		BranchID:           p.BranchID,
		Path:               p.Path,
		LastSeenCommit:     p.LastSeenCommit,
		ParsedAt:           now,
		FeatureName:        p.FeatureName,
		FeatureDescription: p.FeatureDescription,
		FeatureVersion:     featureVersion,
		RawContent:         p.RawContent,
		Requirements:       string(reqsRaw),
		InsertedAt:         now,
		UpdatedAt:          now,
	})
	if err != nil {
		return nil, false, fmt.Errorf("specs: UpsertSpec upsert: %w", err)
	}

	spec, err := specFromRow(row)
	if err != nil {
		return nil, false, err
	}
	return spec, !existed, nil
}

// UpsertFeatureBranchRef writes the refs for (branchID, featureName).
// When override is false, existing refs are merged with incoming (incoming keys win).
// When override is true, incoming refs fully replace any existing row.
func (r *Repository) UpsertFeatureBranchRef(ctx context.Context, branchID, featureName string, refs map[string][]CodeRef, commit string, override bool) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	qr := sqlc.New(r.db.Read)
	qw := sqlc.New(r.db.Write)

	finalRefs := refs
	if !override {
		// Fetch existing row and merge — existing keys not in incoming survive.
		existing, err := qr.GetFeatureBranchRef(ctx, sqlc.GetFeatureBranchRefParams{
			BranchID:    branchID,
			FeatureName: featureName,
		})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("specs: UpsertFeatureBranchRef get: %w", err)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			existingRow, err2 := refsFromRow(existing)
			if err2 != nil {
				return err2
			}
			merged := make(map[string][]CodeRef, len(existingRow.Refs)+len(refs))
			maps.Copy(merged, existingRow.Refs)
			// incoming overwrites
			maps.Copy(merged, refs)
			finalRefs = merged
		}
	}

	// Marshal to JSON.
	type codeRefJSON struct {
		Path   string `json:"path"`
		IsTest bool   `json:"is_test"`
	}
	out := make(map[string][]codeRefJSON, len(finalRefs))
	for k, crefs := range finalRefs {
		arr := make([]codeRefJSON, 0, len(crefs))
		for _, cr := range crefs {
			arr = append(arr, codeRefJSON(cr))
		}
		out[k] = arr
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("specs: UpsertFeatureBranchRef marshal: %w", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("specs: UpsertFeatureBranchRef uuid: %w", err)
	}

	return qw.UpsertFeatureBranchRef(ctx, sqlc.UpsertFeatureBranchRefParams{
		ID:          id.String(),
		BranchID:    branchID,
		FeatureName: featureName,
		Refs:        string(raw),
		Commit:      commit,
		PushedAt:    now,
		InsertedAt:  now,
		UpdatedAt:   now,
	})
}

// ListDistinctFeatureNamesForProduct returns the distinct feature_name values
// across all specs under productID, sorted ascending.
func (r *Repository) ListDistinctFeatureNamesForProduct(ctx context.Context, productID string) ([]string, error) {
	q := sqlc.New(r.db.Read)
	rows, err := q.ListDistinctFeatureNamesForProduct(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("specs: ListDistinctFeatureNamesForProduct: %w", err)
	}
	return rows, nil
}

// UpsertTrackedBranch inserts a tracked_branches row for (implID, branchID) if not
// already present. Silently no-ops if the row exists.
func (r *Repository) UpsertTrackedBranch(ctx context.Context, implID, branchID, repoURI string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	qw := sqlc.New(r.db.Write)
	if err := qw.UpsertTrackedBranch(ctx, sqlc.UpsertTrackedBranchParams{
		ImplementationID: implID,
		BranchID:         branchID,
		RepoUri:          repoURI,
		InsertedAt:       now,
		UpdatedAt:        now,
	}); err != nil {
		return fmt.Errorf("specs: UpsertTrackedBranch: %w", err)
	}
	return nil
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
