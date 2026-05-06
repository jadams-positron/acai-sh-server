package handlers_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

// getCSRFToken GETs path with the given client, asserts 200, and returns both
// the CSRF token from the form body and a new client that carries the CSRF
// cookie from the response (required by Echo's CSRF middleware on the POST).
func getCSRFToken(t *testing.T, client *testfx.Client, path string) (string, *testfx.Client) {
	t.Helper()
	resp := client.GET(path, nil)
	resp.AssertStatus(http.StatusOK)
	csrfToken := extractCSRFToken(string(resp.Body()))
	if csrfToken == "" {
		t.Fatalf("CSRF token not found at %s; body=%.500s", path, resp.Body())
	}
	// Carry the _acai_csrf cookie into subsequent requests.
	clientWithCSRF := client.WithResponseCookies(resp)
	return csrfToken, clientWithCSRF
}

func TestTeamsIndex_EmptyState(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/teams", nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	if !strings.Contains(body, "No teams yet") {
		t.Errorf("expected 'No teams yet' in body; got: %.500s", body)
	}
}

func TestTeamsIndex_ListsUsersTeams(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team1 := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "alpha"})
	team2 := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "bravo"})
	testfx.SeedUserTeamRole(t, app.DB, user, team1, "owner")
	testfx.SeedUserTeamRole(t, app.DB, user, team2, "member")

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/teams", nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	if !strings.Contains(body, "alpha") || !strings.Contains(body, "bravo") {
		t.Errorf("expected both teams in body; got: %.500s", body)
	}
	// Should not show empty state.
	if strings.Contains(body, "No teams yet") {
		t.Errorf("expected no empty-state copy; got: %.500s", body)
	}
}

func TestTeamsIndex_OnlyShowsOwnTeams(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "user1-only@test.example"})
	other := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "user2-only@test.example"})
	myTeam := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "mine"})
	theirTeam := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "notmine"})
	testfx.SeedUserTeamRole(t, app.DB, user, myTeam, "owner")
	testfx.SeedUserTeamRole(t, app.DB, other, theirTeam, "owner")

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/teams", nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	if !strings.Contains(body, "mine") {
		t.Errorf("expected own team 'mine' in body; got: %.500s", body)
	}
	if strings.Contains(body, "notmine") {
		t.Errorf("expected other user's team 'notmine' NOT in body; got: %.500s", body)
	}
}

func TestTeamsIndex_UnauthenticatedRedirects(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	client := app.Client()
	resp := client.GET("/teams", nil)
	if resp.Status() != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect for unauthenticated; got %d", resp.Status())
	}
	if loc := resp.Header("Location"); loc != "/users/log-in" {
		t.Errorf("Location = %q, want /users/log-in", loc)
	}
}

func TestTeamsCreate_HappyPath(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	client := testfx.LoggedInClient(t, app, user)

	csrfToken, postClient := getCSRFToken(t, client, "/teams")

	resp := postClient.POSTForm("/teams", url.Values{
		"name":               {"newteam"},
		"gorilla.csrf.Token": {csrfToken},
	})
	if resp.Status() != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect; got %d, body=%.500s", resp.Status(), resp.Body())
	}
	if loc := resp.Header("Location"); loc != "/t/newteam" {
		t.Errorf("Location = %q, want /t/newteam", loc)
	}

	// Verify team exists in DB and user is linked as owner.
	teamsRepo := teams.NewRepository(app.DB)
	list, err := teamsRepo.ListForUser(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(list) != 1 || list[0].Name != "newteam" {
		t.Errorf("ListForUser = %+v, want [{Name:newteam}]", list)
	}
}

func TestTeamsCreate_InvalidName_422(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	client := testfx.LoggedInClient(t, app, user)

	csrfToken, postClient := getCSRFToken(t, client, "/teams")

	resp := postClient.POSTForm("/teams", url.Values{
		"name":               {"has spaces!"},
		"gorilla.csrf.Token": {csrfToken},
	})
	if resp.Status() != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422; got %d, body=%.500s", resp.Status(), resp.Body())
	}
	body := string(resp.Body())
	if !strings.Contains(body, "alphanumeric") {
		t.Errorf("expected error message about alphanumeric; got: %.500s", body)
	}
}

func TestTeamsCreate_DuplicateName_422(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	// Pre-seed a team with the name.
	testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "exists"})

	client := testfx.LoggedInClient(t, app, user)
	csrfToken, postClient := getCSRFToken(t, client, "/teams")

	resp := postClient.POSTForm("/teams", url.Values{
		"name":               {"exists"},
		"gorilla.csrf.Token": {csrfToken},
	})
	if resp.Status() != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422; got %d, body=%.500s", resp.Status(), resp.Body())
	}
	body := string(resp.Body())
	if !strings.Contains(body, "already exists") {
		t.Errorf("expected 'already exists' error; got: %.500s", body)
	}
}

func TestTeamsCreate_PrefilledNameOnError(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	client := testfx.LoggedInClient(t, app, user)

	csrfToken, postClient := getCSRFToken(t, client, "/teams")

	resp := postClient.POSTForm("/teams", url.Values{
		"name":               {"bad name!"},
		"gorilla.csrf.Token": {csrfToken},
	})
	if resp.Status() != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422; got %d", resp.Status())
	}
	// The errored input value should be prefilled in the re-rendered form.
	body := string(resp.Body())
	if !strings.Contains(body, "bad name!") {
		t.Errorf("expected prefilled name 'bad name!' in response; got: %.500s", body)
	}
}
