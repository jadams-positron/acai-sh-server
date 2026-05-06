package testfx

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

// SeedUserOpts overrides defaults for SeedUser.
type SeedUserOpts struct {
	Email          string // default: "user-<random>@test.example"
	HashedPassword string // default: ""
}

// SeedUser inserts a User with sensible defaults; opts override per-field.
func SeedUser(t *testing.T, db *store.DB, opts SeedUserOpts) *accounts.User {
	t.Helper()
	if opts.Email == "" {
		opts.Email = "user-" + shortID() + "@test.example"
	}
	repo := accounts.NewRepository(db)
	u, err := repo.CreateUser(context.Background(), accounts.CreateUserParams{
		Email:          opts.Email,
		HashedPassword: opts.HashedPassword,
	})
	if err != nil {
		t.Fatalf("testfx.SeedUser: %v", err)
	}
	return u
}

// SeedTeamOpts overrides defaults for SeedTeam.
type SeedTeamOpts struct {
	Name string // default: "team-<random>"
}

// SeedTeam inserts a Team with sensible defaults; opts override per-field.
func SeedTeam(t *testing.T, db *store.DB, opts SeedTeamOpts) *teams.Team {
	t.Helper()
	if opts.Name == "" {
		opts.Name = "team-" + shortID()
	}
	repo := teams.NewRepository(db)
	team, err := repo.CreateTeam(context.Background(), opts.Name)
	if err != nil {
		t.Fatalf("testfx.SeedTeam: %v", err)
	}
	return team
}

// SeedAccessTokenOpts overrides defaults for SeedAccessToken.
type SeedAccessTokenOpts struct {
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

// SeedAccessToken seeds an access token for (user, team) and returns both the
// row AND the plaintext token (the only time the secret is in cleartext).
//
//nolint:gocritic // unnamedResult: naming the returns here would require naked returns, which are less clear
func SeedAccessToken(t *testing.T, db *store.DB, user *accounts.User, team *teams.Team, opts SeedAccessTokenOpts) (*teams.AccessToken, string) {
	t.Helper()
	if opts.Name == "" {
		opts.Name = "test-token-" + shortID()
	}
	repo := teams.NewRepository(db)
	plaintext, err := repo.CreateAccessToken(context.Background(), teams.CreateAccessTokenParams{
		UserID:    user.ID,
		TeamID:    team.ID,
		Name:      opts.Name,
		Scopes:    opts.Scopes,
		ExpiresAt: opts.ExpiresAt,
	})
	if err != nil {
		t.Fatalf("testfx.SeedAccessToken: %v", err)
	}
	prefix, _, _ := strings.Cut(plaintext, ".")
	tok, _, err := repo.VerifyAccessToken(context.Background(), plaintext)
	if err != nil {
		t.Fatalf("testfx.SeedAccessToken: VerifyAccessToken: %v", err)
	}
	_ = prefix
	return tok, plaintext
}

// SeedProductOpts overrides defaults for SeedProduct.
type SeedProductOpts struct {
	Name string
}

// SeededProduct is a thin record used by other seeders. Until a real
// domain/products write path exists, we shape it just enough to chain into
// SeedImplementation, etc.
type SeededProduct struct {
	ID     string
	TeamID string
	Name   string
}

// SeedProduct inserts a product belonging to team. (Direct SQL for now —
// there's no domain/products write path yet beyond reads.)
func SeedProduct(t *testing.T, db *store.DB, team *teams.Team, opts SeedProductOpts) *SeededProduct {
	t.Helper()
	if opts.Name == "" {
		opts.Name = "prod-" + shortID()
	}
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Write.ExecContext(context.Background(),
		"INSERT INTO products (id, team_id, name, is_active, inserted_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)",
		id, team.ID, opts.Name, now, now); err != nil {
		t.Fatalf("testfx.SeedProduct: %v", err)
	}
	return &SeededProduct{ID: id, TeamID: team.ID, Name: opts.Name}
}

// SeedImplementationOpts overrides defaults for SeedImplementation.
type SeedImplementationOpts struct {
	Name string
}

// SeededImplementation is the thin record returned by SeedImplementation.
type SeededImplementation struct {
	ID        string
	ProductID string
	TeamID    string
	Name      string
}

// SeedImplementation inserts an implementation under product.
func SeedImplementation(t *testing.T, db *store.DB, product *SeededProduct, opts SeedImplementationOpts) *SeededImplementation {
	t.Helper()
	if opts.Name == "" {
		opts.Name = "impl-" + shortID()
	}
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Write.ExecContext(context.Background(),
		"INSERT INTO implementations (id, product_id, team_id, name, is_active, inserted_at, updated_at) VALUES (?, ?, ?, ?, 1, ?, ?)",
		id, product.ID, product.TeamID, opts.Name, now, now); err != nil {
		t.Fatalf("testfx.SeedImplementation: %v", err)
	}
	return &SeededImplementation{ID: id, ProductID: product.ID, TeamID: product.TeamID, Name: opts.Name}
}

// SeedBranchOpts overrides defaults for SeedBranch.
type SeedBranchOpts struct {
	RepoURI        string
	BranchName     string
	LastSeenCommit string
}

// SeededBranch is the thin record returned by SeedBranch.
type SeededBranch struct {
	ID, TeamID, RepoURI, BranchName, LastSeenCommit string
}

// SeedBranch inserts a branch row.
func SeedBranch(t *testing.T, db *store.DB, team *teams.Team, opts SeedBranchOpts) *SeededBranch {
	t.Helper()
	if opts.RepoURI == "" {
		opts.RepoURI = "github.com/test/repo-" + shortID()
	}
	if opts.BranchName == "" {
		opts.BranchName = "main"
	}
	if opts.LastSeenCommit == "" {
		opts.LastSeenCommit = "abc123"
	}
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Write.ExecContext(context.Background(),
		"INSERT INTO branches (id, team_id, repo_uri, branch_name, last_seen_commit, inserted_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		id, team.ID, opts.RepoURI, opts.BranchName, opts.LastSeenCommit, now, now); err != nil {
		t.Fatalf("testfx.SeedBranch: %v", err)
	}
	return &SeededBranch{ID: id, TeamID: team.ID, RepoURI: opts.RepoURI, BranchName: opts.BranchName, LastSeenCommit: opts.LastSeenCommit}
}

// SeedTrackedBranch links impl ↔ branch.
func SeedTrackedBranch(t *testing.T, db *store.DB, impl *SeededImplementation, branch *SeededBranch) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Write.ExecContext(context.Background(),
		"INSERT INTO tracked_branches (implementation_id, branch_id, repo_uri, inserted_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		impl.ID, branch.ID, branch.RepoURI, now, now); err != nil {
		t.Fatalf("testfx.SeedTrackedBranch: %v", err)
	}
}

// SeedSpecOpts overrides defaults for SeedSpec.
type SeedSpecOpts struct {
	FeatureName    string
	Description    string
	FeatureVersion string
	Path           string
	LastSeenCommit string
	RawContent     string
	Requirements   map[string]any
}

// SeededSpec is the thin record returned by SeedSpec.
type SeededSpec struct {
	ID, ProductID, BranchID, FeatureName string
}

// SeedSpec inserts a spec row for (product, branch) with feature_name. The
// requirements field is a JSON string; pass a Go map and it'll be marshaled.
func SeedSpec(t *testing.T, db *store.DB, product *SeededProduct, branch *SeededBranch, opts SeedSpecOpts) *SeededSpec {
	t.Helper()
	if opts.FeatureName == "" {
		opts.FeatureName = "feat-" + shortID()
	}
	if opts.LastSeenCommit == "" {
		opts.LastSeenCommit = branch.LastSeenCommit
	}
	if opts.FeatureVersion == "" {
		opts.FeatureVersion = "1.0.0"
	}
	if opts.Requirements == nil {
		opts.Requirements = map[string]any{}
	}
	reqsJSON, err := json.Marshal(opts.Requirements)
	if err != nil {
		t.Fatalf("testfx.SeedSpec: marshal requirements: %v", err)
	}
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Write.ExecContext(context.Background(),
		`INSERT INTO specs (
			id, product_id, branch_id, path, last_seen_commit, parsed_at,
			feature_name, feature_description, feature_version, raw_content, requirements,
			inserted_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, product.ID, branch.ID, opts.Path, opts.LastSeenCommit, now,
		opts.FeatureName, opts.Description, opts.FeatureVersion, opts.RawContent, string(reqsJSON),
		now, now); err != nil {
		t.Fatalf("testfx.SeedSpec: %v", err)
	}
	return &SeededSpec{
		ID: id, ProductID: product.ID, BranchID: branch.ID, FeatureName: opts.FeatureName,
	}
}

// SeedFeatureImplState inserts a feature_impl_states row for impl/feature with
// the given states map (JSON).
func SeedFeatureImplState(t *testing.T, db *store.DB, impl *SeededImplementation, featureName string, states map[string]any) {
	t.Helper()
	if states == nil {
		states = map[string]any{}
	}
	statesJSON, err := json.Marshal(states)
	if err != nil {
		t.Fatalf("testfx.SeedFeatureImplState: marshal: %v", err)
	}
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Write.ExecContext(context.Background(),
		`INSERT INTO feature_impl_states (id, implementation_id, feature_name, states, inserted_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, impl.ID, featureName, string(statesJSON), now, now); err != nil {
		t.Fatalf("testfx.SeedFeatureImplState: %v", err)
	}
}

// SeedFeatureBranchRef inserts a feature_branch_refs row for branch/feature.
func SeedFeatureBranchRef(t *testing.T, db *store.DB, branch *SeededBranch, featureName string, refs map[string]any) {
	t.Helper()
	if refs == nil {
		refs = map[string]any{}
	}
	refsJSON, err := json.Marshal(refs)
	if err != nil {
		t.Fatalf("testfx.SeedFeatureBranchRef: marshal: %v", err)
	}
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Write.ExecContext(context.Background(),
		`INSERT INTO feature_branch_refs (id, branch_id, feature_name, refs, "commit", pushed_at, inserted_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, branch.ID, featureName, string(refsJSON), branch.LastSeenCommit, now, now, now); err != nil {
		t.Fatalf("testfx.SeedFeatureBranchRef: %v", err)
	}
}

// shortID returns a short URL-safe random ID used for default names.
func shortID() string {
	id, _ := uuid.NewV7()
	s := id.String()
	return s[:8]
}
