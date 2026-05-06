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

// settingsURL returns the settings path for a team name.
func settingsURL(teamName string) string { return "/t/" + teamName + "/settings" }

func TestTeamSettings_RendersForOwner(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "settings-owner@test.example"})
	member := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "settings-member@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "stgteam"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")
	testfx.SeedUserTeamRole(t, app.DB, member, team, "member")

	client := testfx.LoggedInClient(t, app, owner)
	resp := client.GET(settingsURL("stgteam"), nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	if !strings.Contains(body, "settings-owner@test.example") {
		t.Errorf("expected owner email in body; got: %.500s", body)
	}
	if !strings.Contains(body, "settings-member@test.example") {
		t.Errorf("expected member email in body; got: %.500s", body)
	}
	if !strings.Contains(body, "Invite Member") {
		t.Errorf("expected 'Invite Member' button in body for owner; got: %.500s", body)
	}
}

func TestTeamSettings_AddMember_HappyPath(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "add-owner@test.example"})
	newMember := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "add-newmember@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "addmbrteam"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")

	client := testfx.LoggedInClient(t, app, owner)
	csrfToken, postClient := getCSRFToken(t, client, settingsURL("addmbrteam"))

	resp := postClient.POSTForm("/t/addmbrteam/settings/members", url.Values{
		"email":              {newMember.Email},
		"role":               {"developer"},
		"gorilla.csrf.Token": {csrfToken},
	})
	if resp.Status() != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect; got %d, body=%.500s", resp.Status(), resp.Body())
	}
	if loc := resp.Header("Location"); loc != settingsURL("addmbrteam") {
		t.Errorf("Location = %q, want %s", loc, settingsURL("addmbrteam"))
	}

	// Verify in DB.
	repo := teams.NewRepository(app.DB)
	members, err := repo.ListMembers(context.Background(), team.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	found := false
	for _, m := range members {
		if m.UserID == newMember.ID && m.Role == "developer" {
			found = true
		}
	}
	if !found {
		t.Errorf("new member not found in members list; got: %+v", members)
	}
}

func TestTeamSettings_AddMember_UnknownEmail_422(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "unk-owner@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "unkemailteam"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")

	client := testfx.LoggedInClient(t, app, owner)
	csrfToken, postClient := getCSRFToken(t, client, settingsURL("unkemailteam"))

	resp := postClient.POSTForm("/t/unkemailteam/settings/members", url.Values{
		"email":              {"doesnotexist@test.example"},
		"role":               {"member"},
		"gorilla.csrf.Token": {csrfToken},
	})
	if resp.Status() != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422; got %d, body=%.500s", resp.Status(), resp.Body())
	}
	body := string(resp.Body())
	if !strings.Contains(body, "register") {
		t.Errorf("expected 'register' in flash; got: %.500s", body)
	}
}

func TestTeamSettings_RemoveMember_HappyPath(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "rm-owner@test.example"})
	member := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "rm-member@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "rmteam"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")
	testfx.SeedUserTeamRole(t, app.DB, member, team, "member")

	client := testfx.LoggedInClient(t, app, owner)
	csrfToken, postClient := getCSRFToken(t, client, settingsURL("rmteam"))

	resp := postClient.POSTForm("/t/rmteam/settings/members/"+member.ID+"/remove", url.Values{
		"gorilla.csrf.Token": {csrfToken},
	})
	if resp.Status() != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect; got %d, body=%.500s", resp.Status(), resp.Body())
	}

	// Verify the member is gone.
	repo := teams.NewRepository(app.DB)
	members, err := repo.ListMembers(context.Background(), team.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	for _, m := range members {
		if m.UserID == member.ID {
			t.Errorf("expected member to be removed; still present: %+v", m)
		}
	}
}

func TestTeamSettings_RemoveLastOwner_422(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "lastowner@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "lastownerteam"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")

	client := testfx.LoggedInClient(t, app, owner)
	csrfToken, postClient := getCSRFToken(t, client, settingsURL("lastownerteam"))

	resp := postClient.POSTForm("/t/lastownerteam/settings/members/"+owner.ID+"/remove", url.Values{
		"gorilla.csrf.Token": {csrfToken},
	})
	if resp.Status() != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422; got %d, body=%.500s", resp.Status(), resp.Body())
	}
	body := string(resp.Body())
	if !strings.Contains(body, "last owner") {
		t.Errorf("expected 'last owner' in flash; got: %.500s", body)
	}
}

func TestTeamSettings_404ForNonMember(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	outsider := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "outsider-stg@test.example"})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "owner-stg@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "secretstg"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")

	client := testfx.LoggedInClient(t, app, outsider)
	resp := client.GET(settingsURL("secretstg"), nil)
	if resp.Status() != http.StatusNotFound {
		t.Fatalf("expected 404 for non-member; got %d", resp.Status())
	}
}
