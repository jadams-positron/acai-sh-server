package handlers_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

func TestImplShow_RendersImplWithFeatures(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "impl-show@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "implshow-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "ledger"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "production"})
	branch := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{
		RepoURI:        "github.com/acme/ledger",
		BranchName:     "main",
		LastSeenCommit: "abcdef0123456789abcdef0123456789abcdef01",
	})
	testfx.SeedTrackedBranch(t, app.DB, impl, branch)
	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{FeatureName: "feature-alpha"})
	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{FeatureName: "feature-beta"})

	slug := makeImplSlug(impl)
	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/implshow-team/i/"+slug, nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	for _, want := range []string{
		`>production<`,              // impl name
		`>ledger<`,                  // product link text
		`/t/implshow-team/p/ledger`, // product link
		`feature-alpha`,
		`feature-beta`,
		`/t/implshow-team/i/` + slug + `/f/feature-alpha`, // feature drill-down
		`/t/implshow-team/i/` + slug + `/f/feature-beta`,
		`root impl`,        // no parent
		`Tracked branches`, // new section header
		`>main</a>`,        // branch name link
		`acme/ledger`,      // repo (after shortenRepoURI)
		`abcdef0`,          // short commit
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected body to contain %q; got: %.800s", want, body)
		}
	}
}

func TestImplShow_NoTrackedBranches_HidesSection(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "impl-untracked@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "impluntrack-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "ledger"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "production"})

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/impluntrack-team/i/"+makeImplSlug(impl), nil)
	resp.AssertStatus(http.StatusOK)

	if strings.Contains(string(resp.Body()), "Tracked branches") {
		t.Errorf("expected no 'Tracked branches' section when impl has no branches; got: %.500s", resp.Body())
	}
}

func TestImplShow_NoFeaturesEmptyState(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "impl-show-empty@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "implshowempty-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "ledger"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "production"})

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/implshowempty-team/i/"+makeImplSlug(impl), nil)
	resp.AssertStatus(http.StatusOK)

	if !strings.Contains(string(resp.Body()), "No features yet") {
		t.Errorf("expected 'No features yet' empty state; got: %.500s", resp.Body())
	}
}

func TestImplShow_404ForBadSlug(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "impl-badslug@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "badslug-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/badslug-team/i/totally-not-a-slug", nil)
	if resp.Status() != http.StatusNotFound {
		t.Fatalf("expected 404 for bad slug; got %d", resp.Status())
	}
}

func TestImplShow_404ForNonMember(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "impl-outsider@test.example"})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "impl-show-owner@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "secretimpl-team"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "p"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "i"})

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/secretimpl-team/i/"+makeImplSlug(impl), nil)
	if resp.Status() != http.StatusNotFound {
		t.Fatalf("expected 404 for non-member; got %d", resp.Status())
	}
}
