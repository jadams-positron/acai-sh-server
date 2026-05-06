package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/store"
	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

// --- helpers ---

// pushResp decodes the push response shape.
type pushResp struct {
	Data struct {
		BranchID           string   `json:"branch_id"`
		ImplementationID   *string  `json:"implementation_id"`
		ImplementationName *string  `json:"implementation_name"`
		ProductName        *string  `json:"product_name"`
		SpecsCreated       int      `json:"specs_created"`
		SpecsUpdated       int      `json:"specs_updated"`
		Warnings           []string `json:"warnings"`
	} `json:"data"`
}

// pushBody is the minimal request body shape (mirrors spec.PushRequest).
type pushBody struct {
	BranchName     string       `json:"branch_name"`
	CommitHash     string       `json:"commit_hash"`
	RepoURI        string       `json:"repo_uri"`
	ProductName    *string      `json:"product_name,omitempty"`
	TargetImplName *string      `json:"target_impl_name,omitempty"`
	Specs          []pushSpec   `json:"specs,omitempty"`
	References     *refsPayload `json:"references,omitempty"`
}

type pushSpec struct {
	Feature      pushFeature          `json:"feature"`
	Meta         pushMeta             `json:"meta"`
	Requirements map[string]pushReqDf `json:"requirements"`
}

type pushFeature struct {
	Name    string  `json:"name"`
	Product string  `json:"product"`
	Version *string `json:"version,omitempty"`
}

type pushMeta struct {
	Path           string  `json:"path"`
	LastSeenCommit string  `json:"last_seen_commit"`
	RawContent     *string `json:"raw_content,omitempty"`
}

type pushReqDf struct {
	Requirement string `json:"requirement"`
}

type refsPayload struct {
	Override *bool                       `json:"override,omitempty"`
	Data     map[string][]codeRefPayload `json:"data"`
}

type codeRefPayload struct {
	Path   string `json:"path"`
	IsTest *bool  `json:"is_test,omitempty"`
}

// readBranchID returns the branch.id for (teamID, repoURI, branchName).
func readBranchID(t *testing.T, db *store.DB, teamID, repoURI, branchName string) string {
	t.Helper()
	var id string
	err := db.Read.QueryRowContext(context.Background(),
		"SELECT id FROM branches WHERE team_id = ? AND repo_uri = ? AND branch_name = ?",
		teamID, repoURI, branchName).Scan(&id)
	if err != nil {
		t.Fatalf("readBranchID: %v", err)
	}
	return id
}

// readSpecRequirements reads the requirements JSON for (branchID, featureName).
func readSpecRequirements(t *testing.T, db *store.DB, branchID, featureName string) map[string]any {
	t.Helper()
	var raw string
	err := db.Read.QueryRowContext(context.Background(),
		"SELECT requirements FROM specs WHERE branch_id = ? AND feature_name = ?",
		branchID, featureName).Scan(&raw)
	if err != nil {
		t.Fatalf("readSpecRequirements: %v", err)
	}
	var out map[string]any
	if err2 := json.Unmarshal([]byte(raw), &out); err2 != nil {
		t.Fatalf("readSpecRequirements unmarshal: %v", err2)
	}
	return out
}

// trackedBranchExists returns true if a tracked_branches row exists.
func trackedBranchExists(t *testing.T, db *store.DB, implID, branchID string) bool {
	t.Helper()
	var n int
	err := db.Read.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM tracked_branches WHERE implementation_id = ? AND branch_id = ?",
		implID, branchID).Scan(&n)
	if err != nil {
		t.Fatalf("trackedBranchExists: %v", err)
	}
	return n > 0
}

// readFeatureRefs reads the refs JSON for (branchID, featureName).
func readFeatureRefs(t *testing.T, db *store.DB, branchID, featureName string) map[string]any {
	t.Helper()
	var raw string
	err := db.Read.QueryRowContext(context.Background(),
		"SELECT refs FROM feature_branch_refs WHERE branch_id = ? AND feature_name = ?",
		branchID, featureName).Scan(&raw)
	if err != nil {
		t.Fatalf("readFeatureRefs: %v", err)
	}
	var out map[string]any
	if err2 := json.Unmarshal([]byte(raw), &out); err2 != nil {
		t.Fatalf("readFeatureRefs unmarshal: %v", err2)
	}
	return out
}

// --- fixture ---

// pushFixture is the shared fixture for push tests.
type pushFixture struct {
	app       *testfx.App
	plaintext string
	team      *teams.Team
	product   *testfx.SeededProduct
	impl      *testfx.SeededImplementation
}

func setupPush(t *testing.T) *pushFixture {
	t.Helper()
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "push-team"})
	_, plaintext := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "myproduct"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "production"})
	return &pushFixture{
		app:       app,
		plaintext: plaintext,
		team:      team,
		product:   product,
		impl:      impl,
	}
}

// --- tests ---

func TestPush_SpecsOnly_Creates(t *testing.T) {
	fx := setupPush(t)

	body := pushBody{
		BranchName: "main",
		CommitHash: "abc1234",
		RepoURI:    "github.com/test/repo",
		Specs: []pushSpec{
			{
				Feature:      pushFeature{Name: "auth-feature", Product: "myproduct"},
				Meta:         pushMeta{Path: "features/auth.yaml", LastSeenCommit: "abc1234"},
				Requirements: map[string]pushReqDf{"auth-feature.AUTH.1": {Requirement: "Users can log in"}},
			},
		},
	}

	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusOK)

	var doc pushResp
	resp.JSON(&doc)

	if doc.Data.SpecsCreated != 1 {
		t.Errorf("specs_created = %d, want 1", doc.Data.SpecsCreated)
	}
	if doc.Data.SpecsUpdated != 0 {
		t.Errorf("specs_updated = %d, want 0", doc.Data.SpecsUpdated)
	}
	if doc.Data.BranchID == "" {
		t.Error("branch_id should be set")
	}
	if doc.Data.ProductName == nil || *doc.Data.ProductName != "myproduct" {
		t.Errorf("product_name = %v, want myproduct", doc.Data.ProductName)
	}

	// Verify branch row exists in DB.
	branchID := readBranchID(t, fx.app.DB, fx.team.ID, "github.com/test/repo", "main")
	if branchID == "" {
		t.Error("branch row should exist in DB")
	}
	if branchID != doc.Data.BranchID {
		t.Errorf("branch_id in response %q != DB %q", doc.Data.BranchID, branchID)
	}
}

func TestPush_SpecsOnly_UpdatesExisting(t *testing.T) {
	fx := setupPush(t)

	// Seed a branch and spec.
	branch := testfx.SeedBranch(t, fx.app.DB, fx.team, testfx.SeedBranchOpts{
		RepoURI:        "github.com/test/repo",
		BranchName:     "main",
		LastSeenCommit: "aabbcc1",
	})
	testfx.SeedSpec(t, fx.app.DB, fx.product, branch, testfx.SeedSpecOpts{
		FeatureName: "auth-feature",
		Requirements: map[string]any{
			"auth-feature.AUTH.1": map[string]any{"requirement": "old requirement"},
		},
	})

	body := pushBody{
		BranchName: "main",
		CommitHash: "ddeeff2",
		RepoURI:    "github.com/test/repo",
		Specs: []pushSpec{
			{
				Feature: pushFeature{Name: "auth-feature", Product: "myproduct"},
				Meta:    pushMeta{Path: "features/auth.yaml", LastSeenCommit: "ddeeff2"},
				Requirements: map[string]pushReqDf{
					"auth-feature.AUTH.1": {Requirement: "Updated: Users can log in"},
					"auth-feature.AUTH.2": {Requirement: "New requirement"},
				},
			},
		},
	}

	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusOK)

	var doc pushResp
	resp.JSON(&doc)

	if doc.Data.SpecsCreated != 0 {
		t.Errorf("specs_created = %d, want 0", doc.Data.SpecsCreated)
	}
	if doc.Data.SpecsUpdated != 1 {
		t.Errorf("specs_updated = %d, want 1", doc.Data.SpecsUpdated)
	}

	reqs := readSpecRequirements(t, fx.app.DB, branch.ID, "auth-feature")
	if _, ok := reqs["auth-feature.AUTH.2"]; !ok {
		t.Error("new requirement auth-feature.AUTH.2 should be in DB")
	}
	entry, _ := reqs["auth-feature.AUTH.1"].(map[string]any)
	if entry["requirement"] != "Updated: Users can log in" {
		t.Errorf("requirement text = %v, want 'Updated: Users can log in'", entry["requirement"])
	}
}

func TestPush_RefsOnly_RequiresImpl(t *testing.T) {
	fx := setupPush(t)

	body := pushBody{
		BranchName:     "main",
		CommitHash:     "abc1234",
		RepoURI:        "github.com/test/repo",
		ProductName:    new("myproduct"),
		TargetImplName: new("production"),
		References: &refsPayload{
			Data: map[string][]codeRefPayload{
				"auth-feature.AUTH.1": {{Path: "lib/auth.go:42"}},
			},
		},
	}

	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusOK)

	var doc pushResp
	resp.JSON(&doc)

	if doc.Data.ImplementationID == nil {
		t.Error("implementation_id should be set")
	}
	if doc.Data.ImplementationName == nil || *doc.Data.ImplementationName != "production" {
		t.Errorf("implementation_name = %v, want production", doc.Data.ImplementationName)
	}

	branchID := readBranchID(t, fx.app.DB, fx.team.ID, "github.com/test/repo", "main")

	if !trackedBranchExists(t, fx.app.DB, fx.impl.ID, branchID) {
		t.Error("tracked_branches row should exist")
	}
	refs := readFeatureRefs(t, fx.app.DB, branchID, "auth-feature")
	if len(refs) == 0 {
		t.Error("feature_branch_refs should have refs for auth-feature")
	}
}

func TestPush_RefsOverrideTrue(t *testing.T) {
	fx := setupPush(t)

	branch := testfx.SeedBranch(t, fx.app.DB, fx.team, testfx.SeedBranchOpts{
		RepoURI:        "github.com/test/repo",
		BranchName:     "main",
		LastSeenCommit: "aabbcc1",
	})
	testfx.SeedTrackedBranch(t, fx.app.DB, fx.impl, branch)
	testfx.SeedFeatureBranchRef(t, fx.app.DB, branch, "auth-feature", map[string]any{
		"auth-feature.AUTH.1": []map[string]any{{"path": "lib/old.go:1", "is_test": false}},
		"auth-feature.AUTH.2": []map[string]any{{"path": "lib/old.go:99", "is_test": false}},
	})

	// Push with override=true, only provides AUTH.3.
	body := pushBody{
		BranchName:     "main",
		CommitHash:     "ddeeff2",
		RepoURI:        "github.com/test/repo",
		ProductName:    new("myproduct"),
		TargetImplName: new("production"),
		References: &refsPayload{
			Override: new(true),
			Data: map[string][]codeRefPayload{
				"auth-feature.AUTH.3": {{Path: "lib/new.go:10"}},
			},
		},
	}

	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusOK)

	refs := readFeatureRefs(t, fx.app.DB, branch.ID, "auth-feature")
	if _, ok := refs["auth-feature.AUTH.1"]; ok {
		t.Error("AUTH.1 should have been replaced (override=true)")
	}
	if _, ok := refs["auth-feature.AUTH.2"]; ok {
		t.Error("AUTH.2 should have been replaced (override=true)")
	}
	if _, ok := refs["auth-feature.AUTH.3"]; !ok {
		t.Error("AUTH.3 should be present after override")
	}
}

func TestPush_RefsOverrideFalse_Merges(t *testing.T) {
	fx := setupPush(t)

	branch := testfx.SeedBranch(t, fx.app.DB, fx.team, testfx.SeedBranchOpts{
		RepoURI:        "github.com/test/repo",
		BranchName:     "main",
		LastSeenCommit: "aabbcc1",
	})
	testfx.SeedTrackedBranch(t, fx.app.DB, fx.impl, branch)
	testfx.SeedFeatureBranchRef(t, fx.app.DB, branch, "auth-feature", map[string]any{
		"auth-feature.AUTH.1": []map[string]any{{"path": "lib/existing.go:1", "is_test": false}},
	})

	// Push with override=false (merge), adds AUTH.2.
	body := pushBody{
		BranchName:     "main",
		CommitHash:     "ddeeff2",
		RepoURI:        "github.com/test/repo",
		ProductName:    new("myproduct"),
		TargetImplName: new("production"),
		References: &refsPayload{
			Override: new(false),
			Data: map[string][]codeRefPayload{
				"auth-feature.AUTH.2": {{Path: "lib/new.go:55"}},
			},
		},
	}

	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusOK)

	refs := readFeatureRefs(t, fx.app.DB, branch.ID, "auth-feature")
	if _, ok := refs["auth-feature.AUTH.1"]; !ok {
		t.Error("AUTH.1 should survive merge (override=false)")
	}
	if _, ok := refs["auth-feature.AUTH.2"]; !ok {
		t.Error("AUTH.2 should be added by merge")
	}
}

func TestPush_AutoCreatesProduct_OnSpecPush(t *testing.T) {
	fx := setupPush(t)

	// Push a spec referencing a product that does NOT exist yet.
	body := pushBody{
		BranchName: "main",
		CommitHash: "abc1234",
		RepoURI:    "github.com/test/repo",
		Specs: []pushSpec{
			{
				Feature:      pushFeature{Name: "auth-feature", Product: "newprod"},
				Meta:         pushMeta{Path: "features/auth.yaml", LastSeenCommit: "abc1234"},
				Requirements: map[string]pushReqDf{},
			},
		},
	}

	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusOK)

	var doc pushResp
	resp.JSON(&doc)

	if doc.Data.SpecsCreated != 1 {
		t.Errorf("specs_created = %d, want 1", doc.Data.SpecsCreated)
	}
	if doc.Data.ProductName == nil || *doc.Data.ProductName != "newprod" {
		t.Errorf("product_name = %v, want newprod", doc.Data.ProductName)
	}

	// Verify the product row was created in DB.
	var count int
	err := fx.app.DB.Read.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM products WHERE team_id = ? AND name = ?",
		fx.team.ID, "newprod").Scan(&count)
	if err != nil {
		t.Fatalf("DB check: %v", err)
	}
	if count != 1 {
		t.Errorf("DB product count = %d, want 1", count)
	}
}

func TestPush_ImplNotFound_422(t *testing.T) {
	fx := setupPush(t)

	body := pushBody{
		BranchName:     "main",
		CommitHash:     "abc1234",
		RepoURI:        "github.com/test/repo",
		ProductName:    new("myproduct"),
		TargetImplName: new("does-not-exist"),
		References: &refsPayload{
			Data: map[string][]codeRefPayload{
				"auth-feature.AUTH.1": {{Path: "lib/auth.go:1"}},
			},
		},
	}

	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusUnprocessableEntity)
}

func TestPush_TooManySpecs_413(t *testing.T) {
	// Lower the cap via env var, then build a fresh App.
	t.Setenv("API_PUSH_MAX_SPECS", "2")
	app2 := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app2.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app2.DB, testfx.SeedTeamOpts{Name: "team-caps"})
	_, token := testfx.SeedAccessToken(t, app2.DB, user, team, testfx.SeedAccessTokenOpts{})
	testfx.SeedProduct(t, app2.DB, team, testfx.SeedProductOpts{Name: "myproduct"})

	body := pushBody{
		BranchName: "main",
		CommitHash: "abc1234",
		RepoURI:    "github.com/test/repo",
		Specs: []pushSpec{
			{Feature: pushFeature{Name: "feat-a", Product: "myproduct"}, Meta: pushMeta{Path: "a.yaml", LastSeenCommit: "abc1234"}, Requirements: map[string]pushReqDf{}},
			{Feature: pushFeature{Name: "feat-b", Product: "myproduct"}, Meta: pushMeta{Path: "b.yaml", LastSeenCommit: "abc1234"}, Requirements: map[string]pushReqDf{}},
			{Feature: pushFeature{Name: "feat-c", Product: "myproduct"}, Meta: pushMeta{Path: "c.yaml", LastSeenCommit: "abc1234"}, Requirements: map[string]pushReqDf{}},
		},
	}

	resp := app2.Client().WithBearer(token).POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusRequestEntityTooLarge)
}

// TestPush_RefsWithoutProductName_InfersFromTracked verifies that a refs push
// with target_impl_name but no product_name uses the inference path. When the
// branch is not yet in tracked_branches (0 tracking impls), the server returns
// 200 with no implementation_id/product_name (no-impl linkage case).
func TestPush_RefsWithoutProductName_InfersFromTracked(t *testing.T) {
	fx := setupPush(t)

	body := pushBody{
		BranchName:     "untracked-branch",
		CommitHash:     "abc1234",
		RepoURI:        "github.com/test/repo",
		TargetImplName: new("production"),
		// ProductName intentionally absent — triggers inference path.
		References: &refsPayload{
			Data: map[string][]codeRefPayload{
				"auth-feature.AUTH.1": {{Path: "lib/auth.go:1"}},
			},
		},
	}

	// 0 impls track this branch → inference yields no impl linkage → 200.
	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusOK)

	var doc pushResp
	resp.JSON(&doc)
	if doc.Data.ImplementationID != nil {
		t.Errorf("implementation_id = %v, want nil (no tracking impl)", doc.Data.ImplementationID)
	}
	if doc.Data.ProductName != nil {
		t.Errorf("product_name = %v, want nil (no tracking impl)", doc.Data.ProductName)
	}
	// Refs should still be written under the branch.
	branchID := readBranchID(t, fx.app.DB, fx.team.ID, "github.com/test/repo", "untracked-branch")
	refs := readFeatureRefs(t, fx.app.DB, branchID, "auth-feature")
	if len(refs) == 0 {
		t.Error("feature_branch_refs should be written even without impl linkage")
	}
}

func TestPush_NoBearer_401(t *testing.T) {
	fx := setupPush(t)

	body := pushBody{
		BranchName: "main",
		CommitHash: "abc1234",
		RepoURI:    "github.com/test/repo",
	}
	resp := fx.app.Client().POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusUnauthorized)
}

func TestPush_BadJSON_400(t *testing.T) {
	fx := setupPush(t)
	resp := fx.app.Client().WithBearer(fx.plaintext).
		POSTRaw("/api/v1/push", "application/json", strings.NewReader("{invalid json"))
	resp.AssertStatus(http.StatusBadRequest)
}
