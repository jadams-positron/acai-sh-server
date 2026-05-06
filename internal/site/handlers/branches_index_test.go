package handlers_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

func TestBranchesIndex_RendersBranchesWithTrackingImpls(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "branch-idx@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "branchidxteam"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "ledger"})
	implProd := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "production"})
	implStg := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "staging"})

	branchMain := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{
		RepoURI:        "github.com/test/ledger",
		BranchName:     "main",
		LastSeenCommit: "abcdef0123456789abcdef0123456789abcdef01",
	})
	branchFeat := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{
		RepoURI:    "github.com/test/ledger-fork",
		BranchName: "feat-x",
	})
	// Both impls track main; only staging tracks feat-x.
	testfx.SeedTrackedBranch(t, app.DB, implProd, branchMain)
	testfx.SeedTrackedBranch(t, app.DB, implStg, branchMain)
	testfx.SeedTrackedBranch(t, app.DB, implStg, branchFeat)

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/branchidxteam/branches", nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	for _, want := range []string{
		`>Branches<`,          // page header
		`main`,                // branch name
		`feat-x`,              // branch name
		`abcdef0`,             // short commit
		`ledger / production`, // product / impl chip
		`ledger / staging`,    // both chips
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected body to contain %q; got: %.800s", want, body)
		}
	}
}

func TestBranchesIndex_EmptyState(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "branch-empty@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "emptybranchteam"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/emptybranchteam/branches", nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	if !strings.Contains(body, "No branches yet") {
		t.Errorf("expected empty-state message; got: %.500s", body)
	}
}

func TestBranchesIndex_404ForNonMember(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "branch-outsider@test.example"})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "branch-owner@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "secretbranchteam"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/secretbranchteam/branches", nil)
	if resp.Status() != http.StatusNotFound {
		t.Fatalf("expected 404 for non-member; got %d", resp.Status())
	}
}
