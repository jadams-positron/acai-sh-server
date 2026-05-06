package handlers_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

func TestFeaturesIndex_RendersFeaturesAcrossProducts(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "feat-idx@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "featidx-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	prodLedger := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "ledger"})
	prodBilling := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "billing"})
	implLedger := testfx.SeedImplementation(t, app.DB, prodLedger, testfx.SeedImplementationOpts{Name: "production"})
	implBilling := testfx.SeedImplementation(t, app.DB, prodBilling, testfx.SeedImplementationOpts{Name: "production"})
	branchLedger := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{BranchName: "main-ledger"})
	branchBilling := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{BranchName: "main-billing"})
	testfx.SeedTrackedBranch(t, app.DB, implLedger, branchLedger)
	testfx.SeedTrackedBranch(t, app.DB, implBilling, branchBilling)

	// "auth" lives in BOTH products; "ledger-only" lives only in ledger.
	testfx.SeedSpec(t, app.DB, prodLedger, branchLedger, testfx.SeedSpecOpts{FeatureName: "auth"})
	testfx.SeedSpec(t, app.DB, prodLedger, branchLedger, testfx.SeedSpecOpts{FeatureName: "ledger-only"})
	testfx.SeedSpec(t, app.DB, prodBilling, branchBilling, testfx.SeedSpecOpts{FeatureName: "auth"})

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/featidx-team/features", nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	for _, want := range []string{
		`>Features<`, // page header
		`>auth<`,     // feature name
		`>ledger-only<`,
		`/t/featidx-team/f/auth`, // feature link
		`/t/featidx-team/f/ledger-only`,
		`/t/featidx-team/p/ledger`, // product chip link
		`/t/featidx-team/p/billing`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected body to contain %q; got: %.800s", want, body)
		}
	}

	// "auth" should reference both products; "ledger-only" only ledger.
	authIdx := strings.Index(body, `>auth<`)
	ledgerOnlyIdx := strings.Index(body, `>ledger-only<`)
	if authIdx < 0 || ledgerOnlyIdx < 0 {
		t.Fatalf("could not find feature anchors; body: %.800s", body)
	}
	authRow := body[authIdx:ledgerOnlyIdx]
	if !strings.Contains(authRow, "ledger") || !strings.Contains(authRow, "billing") {
		t.Errorf("expected auth row to chip both products; got row:\n%s", authRow)
	}
}

func TestFeaturesIndex_EmptyState(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "feat-empty@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "emptyfeat-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/emptyfeat-team/features", nil)
	resp.AssertStatus(http.StatusOK)

	if !strings.Contains(string(resp.Body()), "No features yet") {
		t.Errorf("expected empty-state message; got: %.500s", resp.Body())
	}
}

func TestFeaturesIndex_404ForNonMember(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "feat-outsider@test.example"})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "feat-owner@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "secretfeat-team"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/secretfeat-team/features", nil)
	if resp.Status() != http.StatusNotFound {
		t.Fatalf("expected 404 for non-member; got %d", resp.Status())
	}
}
