package handlers_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

func TestTeamShow_RendersForMember(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "member@test.example"})
	other := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "other@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "myteam"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")
	testfx.SeedUserTeamRole(t, app.DB, other, team, "member")
	testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "alpha-product"})
	testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "beta-product"})

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/myteam", nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	if !strings.Contains(body, "myteam") {
		t.Errorf("expected team name 'myteam' in body; got: %.500s", body)
	}
	if !strings.Contains(body, "alpha-product") {
		t.Errorf("expected product 'alpha-product' in body; got: %.500s", body)
	}
	if !strings.Contains(body, "beta-product") {
		t.Errorf("expected product 'beta-product' in body; got: %.500s", body)
	}
	if !strings.Contains(body, "member@test.example") {
		t.Errorf("expected member email in body; got: %.500s", body)
	}
	if !strings.Contains(body, "other@test.example") {
		t.Errorf("expected other member email in body; got: %.500s", body)
	}
}

func TestTeamShow_404ForNonMember(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "nonmember@test.example"})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "owner@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "secretteam"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")

	// user is NOT a member of secretteam
	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/secretteam", nil)
	if resp.Status() != http.StatusNotFound {
		t.Fatalf("expected 404 for non-member; got %d, body=%.500s", resp.Status(), resp.Body())
	}
}

func TestTeamShow_404ForUnknownTeam(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/no-such-team", nil)
	if resp.Status() != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown team; got %d", resp.Status())
	}
}

func TestTeamShow_RedirectsAnon(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "anyteam"})
	_ = team

	// unauthenticated client (no session cookie)
	client := app.Client()
	resp := client.GET("/t/anyteam", nil)
	if resp.Status() != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect for anon; got %d", resp.Status())
	}
	if loc := resp.Header("Location"); loc != "/users/log-in" {
		t.Errorf("Location = %q, want /users/log-in", loc)
	}
}

func TestTeamCreateProduct_HappyPath(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "prodteam"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	client := testfx.LoggedInClient(t, app, user)
	csrfToken, postClient := getCSRFToken(t, client, "/t/prodteam")

	resp := postClient.POSTForm("/t/prodteam/products", url.Values{
		"name":               {"newprod"},
		"gorilla.csrf.Token": {csrfToken},
	})
	if resp.Status() != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect after create; got %d, body=%.500s", resp.Status(), resp.Body())
	}
	if loc := resp.Header("Location"); loc != "/t/prodteam/p/newprod" {
		t.Errorf("Location = %q, want /t/prodteam/p/newprod", loc)
	}

	// Verify the product exists in the DB under the correct team.
	repo := products.NewRepository(app.DB)
	prods, err := repo.ListForTeam(context.Background(), team.ID)
	if err != nil {
		t.Fatalf("ListForTeam: %v", err)
	}
	if len(prods) != 1 || prods[0].Name != "newprod" {
		t.Errorf("ListForTeam = %+v, want [{Name:newprod}]", prods)
	}
	if prods[0].TeamID != team.ID {
		t.Errorf("product TeamID = %q, want %q", prods[0].TeamID, team.ID)
	}
}

func TestTeamCreateProduct_InvalidName_422(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "valteam"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	client := testfx.LoggedInClient(t, app, user)
	csrfToken, postClient := getCSRFToken(t, client, "/t/valteam")

	resp := postClient.POSTForm("/t/valteam/products", url.Values{
		"name":               {"has spaces"},
		"gorilla.csrf.Token": {csrfToken},
	})
	if resp.Status() != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid name; got %d, body=%.500s", resp.Status(), resp.Body())
	}
	body := string(resp.Body())
	if !strings.Contains(body, "alphanumeric") {
		t.Errorf("expected error about alphanumeric; got: %.500s", body)
	}
}

func TestTeamCreateProduct_DuplicateName_422(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "dupteam"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")
	testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "exists"})

	client := testfx.LoggedInClient(t, app, user)
	csrfToken, postClient := getCSRFToken(t, client, "/t/dupteam")

	resp := postClient.POSTForm("/t/dupteam/products", url.Values{
		"name":               {"exists"},
		"gorilla.csrf.Token": {csrfToken},
	})
	if resp.Status() != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for duplicate name; got %d, body=%.500s", resp.Status(), resp.Body())
	}
	body := string(resp.Body())
	if !strings.Contains(body, "already exists") {
		t.Errorf("expected 'already exists' message; got: %.500s", body)
	}
}

func TestTeamCreateProduct_NonMember_404(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "nonmember-post@test.example"})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "owner-post@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "restricted"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")

	// user is not a member — grab a CSRF token from another page first
	client := testfx.LoggedInClient(t, app, user)

	// We need a CSRF token but can't GET /t/restricted (would 404 for non-member).
	// Grab one from /teams instead.
	csrfToken, postClient := getCSRFToken(t, client, "/teams")

	resp := postClient.POSTForm("/t/restricted/products", url.Values{
		"name":               {"myprod"},
		"gorilla.csrf.Token": {csrfToken},
	})
	if resp.Status() != http.StatusNotFound {
		t.Fatalf("expected 404 for non-member POST; got %d, body=%.500s", resp.Status(), resp.Body())
	}
}
