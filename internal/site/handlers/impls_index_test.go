package handlers_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

func TestImplsIndex_RendersAllImplsAcrossProducts(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "impls-idx@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "implsidx-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	prodA := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "ledger"})
	prodB := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "billing"})
	implA1 := testfx.SeedImplementation(t, app.DB, prodA, testfx.SeedImplementationOpts{Name: "production"})
	implA2 := testfx.SeedImplementation(t, app.DB, prodA, testfx.SeedImplementationOpts{Name: "staging"})
	implB1 := testfx.SeedImplementation(t, app.DB, prodB, testfx.SeedImplementationOpts{Name: "main"})

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/implsidx-team/implementations", nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	for _, want := range []string{
		`>Implementations<`, // page header
		`ledger`,            // product header
		`billing`,
		`>production<`,
		`>staging<`,
		`>main<`,
		`/t/implsidx-team/i/` + makeImplSlug(implA1),
		`/t/implsidx-team/i/` + makeImplSlug(implA2),
		`/t/implsidx-team/i/` + makeImplSlug(implB1),
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected body to contain %q; got: %.500s", want, body)
		}
	}
}

func TestImplsIndex_EmptyState(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "impls-empty@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "emptyimpls-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/emptyimpls-team/implementations", nil)
	resp.AssertStatus(http.StatusOK)

	if !strings.Contains(string(resp.Body()), "No implementations yet") {
		t.Errorf("expected empty-state message; got: %.500s", resp.Body())
	}
}

func TestImplsIndex_404ForNonMember(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "impls-outsider@test.example"})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "impls-owner@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "secretimpls-team"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/secretimpls-team/implementations", nil)
	if resp.Status() != http.StatusNotFound {
		t.Fatalf("expected 404 for non-member; got %d, body=%.500s", resp.Status(), resp.Body())
	}
}
