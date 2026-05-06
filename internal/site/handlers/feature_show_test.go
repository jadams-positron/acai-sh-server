package handlers_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/store"
	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

// seedChildImplementation inserts an implementation with the given parent.
func seedChildImplementation(t *testing.T, db *store.DB, product *testfx.SeededProduct, name, parentID string) *testfx.SeededImplementation {
	t.Helper()
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Write.ExecContext(context.Background(),
		"INSERT INTO implementations (id, product_id, team_id, name, parent_implementation_id, is_active, inserted_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, ?, ?)",
		id, product.ID, product.TeamID, name, parentID, now, now); err != nil {
		t.Fatalf("seedChildImplementation: %v", err)
	}
	return &testfx.SeededImplementation{ID: id, ProductID: product.ID, TeamID: product.TeamID, Name: name}
}

func TestFeatureShow_RendersImplCards(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "featshow-member@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "featshow-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "featshow-product"})
	impl1 := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "impl-alpha"})
	impl2 := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "impl-beta"})

	branch1 := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{RepoURI: "github.com/test/repo-alpha"})
	branch2 := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{RepoURI: "github.com/test/repo-beta"})
	testfx.SeedTrackedBranch(t, app.DB, impl1, branch1)
	testfx.SeedTrackedBranch(t, app.DB, impl2, branch2)

	featureName := "cool-feature"
	testfx.SeedSpec(t, app.DB, product, branch1, testfx.SeedSpecOpts{
		FeatureName:  featureName,
		Description:  "Cool feature description",
		Requirements: map[string]any{"AC-001": map[string]any{"requirement": "req one"}},
	})
	testfx.SeedSpec(t, app.DB, product, branch2, testfx.SeedSpecOpts{
		FeatureName:  featureName,
		Requirements: map[string]any{"AC-001": map[string]any{"requirement": "req one"}},
	})
	// Add states for impl1.
	testfx.SeedFeatureImplState(t, app.DB, impl1, featureName, map[string]any{
		"AC-001": map[string]any{"status": "completed"},
	})

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/featshow-team/f/"+featureName, nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	if !strings.Contains(body, "cool-feature") {
		t.Errorf("expected feature name in body; got: %.500s", body)
	}
	if !strings.Contains(body, "impl-alpha") {
		t.Errorf("expected impl-alpha in body; got: %.500s", body)
	}
	if !strings.Contains(body, "impl-beta") {
		t.Errorf("expected impl-beta in body; got: %.500s", body)
	}
	if !strings.Contains(body, "featshow-product") {
		t.Errorf("expected product name in body; got: %.500s", body)
	}
}

func TestFeatureShow_HierarchyOrder(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "hierarchy-member@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "hierarchy-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "hierarchy-product"})

	parent := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "parent-impl"})
	child1 := seedChildImplementation(t, app.DB, product, "child-impl-a", parent.ID)
	child2 := seedChildImplementation(t, app.DB, product, "child-impl-b", parent.ID)

	branchP := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{RepoURI: "github.com/test/repo-parent"})
	branchC1 := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{RepoURI: "github.com/test/repo-child1"})
	branchC2 := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{RepoURI: "github.com/test/repo-child2"})
	testfx.SeedTrackedBranch(t, app.DB, parent, branchP)
	testfx.SeedTrackedBranch(t, app.DB, child1, branchC1)
	testfx.SeedTrackedBranch(t, app.DB, child2, branchC2)

	featureName := "hierarchy-feature"
	reqs := map[string]any{"AC-001": map[string]any{"requirement": "req"}}
	testfx.SeedSpec(t, app.DB, product, branchP, testfx.SeedSpecOpts{FeatureName: featureName, Requirements: reqs})
	testfx.SeedSpec(t, app.DB, product, branchC1, testfx.SeedSpecOpts{FeatureName: featureName, Requirements: reqs})
	testfx.SeedSpec(t, app.DB, product, branchC2, testfx.SeedSpecOpts{FeatureName: featureName, Requirements: reqs})

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/hierarchy-team/f/"+featureName, nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())

	posParent := strings.Index(body, "parent-impl")
	posChild1 := strings.Index(body, "child-impl-a")
	posChild2 := strings.Index(body, "child-impl-b")

	if posParent < 0 || posChild1 < 0 || posChild2 < 0 {
		t.Fatalf("expected all impl names in body; parent=%d child1=%d child2=%d", posParent, posChild1, posChild2)
	}
	if posParent > posChild1 || posParent > posChild2 {
		t.Errorf("expected parent-impl to appear before children in body; parent=%d child1=%d child2=%d", posParent, posChild1, posChild2)
	}
}

func TestFeatureShow_EmptyState(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "feat-empty@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "feat-empty-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/feat-empty-team/f/some-feature", nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	if !strings.Contains(body, "No implementations have this feature yet") {
		t.Errorf("expected empty state copy in body; got: %.500s", body)
	}
}

func TestFeatureShow_404ForNonMember(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "feat-nonmember@test.example"})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "feat-owner@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "feat-secret-team"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/feat-secret-team/f/any-feature", nil)
	if resp.Status() != http.StatusNotFound {
		t.Fatalf("expected 404 for non-member; got %d, body=%.500s", resp.Status(), resp.Body())
	}
}

func TestFeatureShow_RedirectsAnon(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	_ = testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "feat-anon-team"})

	client := app.Client()
	resp := client.GET("/t/feat-anon-team/f/some-feature", nil)
	if resp.Status() != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect for anon; got %d", resp.Status())
	}
	if loc := resp.Header("Location"); loc != "/users/log-in" {
		t.Errorf("Location = %q, want /users/log-in", loc)
	}
}

// Compile-time check: ensure the test uses the right types from domain packages.
var _ *accounts.User
var _ *teams.Team
