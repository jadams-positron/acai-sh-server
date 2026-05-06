package handlers_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

func TestProductShow_RendersForMember(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "prod-member@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "prodshow-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "myproduct"})
	impl1 := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "impl-one"})
	impl2 := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "impl-two"})
	branch := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{})
	testfx.SeedTrackedBranch(t, app.DB, impl1, branch)
	testfx.SeedTrackedBranch(t, app.DB, impl2, branch)
	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{FeatureName: "feature-alpha"})
	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{FeatureName: "feature-beta"})

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/prodshow-team/p/myproduct", nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	if !strings.Contains(body, "myproduct") {
		t.Errorf("expected product name 'myproduct' in body; got: %.500s", body)
	}
	if !strings.Contains(body, "impl-one") {
		t.Errorf("expected impl name 'impl-one' in body; got: %.500s", body)
	}
	if !strings.Contains(body, "impl-two") {
		t.Errorf("expected impl name 'impl-two' in body; got: %.500s", body)
	}
	if !strings.Contains(body, "feature-alpha") {
		t.Errorf("expected feature name 'feature-alpha' in body; got: %.500s", body)
	}
	if !strings.Contains(body, "feature-beta") {
		t.Errorf("expected feature name 'feature-beta' in body; got: %.500s", body)
	}
}

func TestProductShow_EmptyState(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "prod-empty@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "empty-prod-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")
	testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "emptyproduct"})

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/empty-prod-team/p/emptyproduct", nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	for _, want := range []string{
		"No implementations yet",
		"No features yet",
		// Both empty states should surface the actual acai push command —
		// the product name interpolated into a copy-pasteable snippet.
		"acai push --product emptyproduct --all",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %q in product-empty body; got: %.800s", want, body)
		}
	}
}

func TestProductShow_404ForNonMember(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "prod-nonmember@test.example"})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "prod-owner@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "secret-prod-team"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")
	testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "secretprod"})

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/secret-prod-team/p/secretprod", nil)
	if resp.Status() != http.StatusNotFound {
		t.Fatalf("expected 404 for non-member; got %d, body=%.500s", resp.Status(), resp.Body())
	}
}

func TestProductShow_404ForUnknownProduct(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "prod-unknown@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "known-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/known-team/p/no-such-product", nil)
	if resp.Status() != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown product; got %d", resp.Status())
	}
}

func TestProductShow_RedirectsAnon(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "anon-prod-team"})
	testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "anon-prod"})

	client := app.Client()
	resp := client.GET("/t/anon-prod-team/p/anon-prod", nil)
	if resp.Status() != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect for anon; got %d", resp.Status())
	}
	if loc := resp.Header("Location"); loc != "/users/log-in" {
		t.Errorf("Location = %q, want /users/log-in", loc)
	}
}
