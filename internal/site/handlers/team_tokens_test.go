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

// tokensURL returns the tokens path for a team name.
func tokensURL(teamName string) string { return "/t/" + teamName + "/tokens" }

func TestTeamTokens_Renders(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "tok-owner@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "tokteam"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")
	tok, _ := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{Name: "ci-token"})

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET(tokensURL("tokteam"), nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	if !strings.Contains(body, "ci-token") {
		t.Errorf("expected token name 'ci-token' in body; got: %.500s", body)
	}
	if !strings.Contains(body, tok.TokenPrefix) {
		t.Errorf("expected token prefix %q in body; got: %.500s", tok.TokenPrefix, body)
	}
	if !strings.Contains(body, "active") {
		t.Errorf("expected status 'active' in body; got: %.500s", body)
	}
}

func TestTeamTokens_Create_HappyPath(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "create-tok@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "createtokteam"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	client := testfx.LoggedInClient(t, app, user)
	csrfToken, postClient := getCSRFToken(t, client, tokensURL("createtokteam"))

	resp := postClient.POSTForm(tokensURL("createtokteam"), url.Values{
		"name":               {"deploy-key"},
		"gorilla.csrf.Token": {csrfToken},
	})
	// After creation, handler renders directly (200) with the plaintext banner.
	if resp.Status() != http.StatusOK {
		t.Fatalf("expected 200 with plaintext banner; got %d, body=%.500s", resp.Status(), resp.Body())
	}
	body := string(resp.Body())
	if !strings.Contains(body, "Save this token") {
		t.Errorf("expected plaintext banner in body; got: %.500s", body)
	}
	if !strings.Contains(body, "deploy-key") {
		t.Errorf("expected token name 'deploy-key' in body; got: %.500s", body)
	}

	// Token must exist in DB.
	repo := teams.NewRepository(app.DB)
	toks, err := repo.ListAccessTokensForTeam(context.Background(), team.ID)
	if err != nil {
		t.Fatalf("ListAccessTokensForTeam: %v", err)
	}
	if len(toks) != 1 || toks[0].Name != "deploy-key" {
		t.Errorf("expected 1 token named 'deploy-key'; got: %+v", toks)
	}
}

func TestTeamTokens_Create_NoName_422(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "noname-tok@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "nonameteam"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	client := testfx.LoggedInClient(t, app, user)
	csrfToken, postClient := getCSRFToken(t, client, tokensURL("nonameteam"))

	resp := postClient.POSTForm(tokensURL("nonameteam"), url.Values{
		"name":               {""},
		"gorilla.csrf.Token": {csrfToken},
	})
	if resp.Status() != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422; got %d, body=%.500s", resp.Status(), resp.Body())
	}
	body := string(resp.Body())
	if !strings.Contains(body, "required") {
		t.Errorf("expected 'required' in flash; got: %.500s", body)
	}
}

func TestTeamTokens_Revoke_HappyPath(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "revoke-owner@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "revoketeam"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")
	tok, _ := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{Name: "to-revoke"})

	client := testfx.LoggedInClient(t, app, user)
	csrfToken, postClient := getCSRFToken(t, client, tokensURL("revoketeam"))

	revokeURL := "/t/revoketeam/tokens/" + tok.TokenPrefix + "/revoke"
	resp := postClient.POSTForm(revokeURL, url.Values{
		"gorilla.csrf.Token": {csrfToken},
	})
	if resp.Status() != http.StatusSeeOther {
		t.Fatalf("expected 303; got %d, body=%.500s", resp.Status(), resp.Body())
	}

	// Token should now show as revoked on the page.
	getResp := client.GET(tokensURL("revoketeam"), nil)
	getResp.AssertStatus(http.StatusOK)
	body := string(getResp.Body())
	if !strings.Contains(body, "revoked") {
		t.Errorf("expected 'revoked' status in body; got: %.500s", body)
	}
}

func TestTeamTokens_Revoke_AlreadyRevoked_OK(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "dblrevoke@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "dblrevoketeam"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")
	tok, _ := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{Name: "already-revoked"})

	// Revoke once first.
	repo := teams.NewRepository(app.DB)
	if err := repo.RevokeAccessTokenByPrefix(context.Background(), tok.TokenPrefix); err != nil {
		t.Fatalf("first revoke: %v", err)
	}

	client := testfx.LoggedInClient(t, app, user)
	csrfToken, postClient := getCSRFToken(t, client, tokensURL("dblrevoketeam"))

	revokeURL := "/t/dblrevoketeam/tokens/" + tok.TokenPrefix + "/revoke"
	resp := postClient.POSTForm(revokeURL, url.Values{
		"gorilla.csrf.Token": {csrfToken},
	})
	// Should still redirect — idempotent.
	if resp.Status() != http.StatusSeeOther {
		t.Fatalf("expected 303 (idempotent); got %d, body=%.500s", resp.Status(), resp.Body())
	}
}

func TestTeamTokens_404ForNonMember(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	outsider := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "outsider-tok@test.example"})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "owner-tok@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "secrettoktm"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")

	client := testfx.LoggedInClient(t, app, outsider)
	resp := client.GET(tokensURL("secrettoktm"), nil)
	if resp.Status() != http.StatusNotFound {
		t.Fatalf("expected 404 for non-member; got %d", resp.Status())
	}
}
