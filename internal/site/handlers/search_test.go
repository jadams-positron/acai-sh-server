package handlers_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

type searchResponse struct {
	Products []struct{ Label, Href, Hint string } `json:"products"`
	Impls    []struct{ Label, Href, Hint string } `json:"impls"`
	Features []struct{ Label, Href, Hint string } `json:"features"`
	Branches []struct{ Label, Href, Hint string } `json:"branches"`
}

func TestSearch_FindsAcrossGroups(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "search@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "searchteam"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	prodLedger := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "ledger-product"})
	testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "billing"})
	implProd := testfx.SeedImplementation(t, app.DB, prodLedger, testfx.SeedImplementationOpts{Name: "ledger-prod"})
	branch := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{
		RepoURI: "github.com/acme/ledger", BranchName: "ledger-feat",
	})
	testfx.SeedTrackedBranch(t, app.DB, implProd, branch)
	testfx.SeedSpec(t, app.DB, prodLedger, branch, testfx.SeedSpecOpts{FeatureName: "ledger-auth"})

	client := testfx.LoggedInClient(t, app, user)

	// Search for "ledger" — should match across all groups.
	resp := client.GET("/t/searchteam/search?q=ledger", nil)
	resp.AssertStatus(http.StatusOK)

	var out searchResponse
	if err := json.Unmarshal(resp.Body(), &out); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, resp.Body())
	}

	if len(out.Products) == 0 || !strings.Contains(out.Products[0].Label, "ledger") {
		t.Errorf("expected ledger product hit; got %+v", out.Products)
	}
	if len(out.Impls) == 0 || !strings.Contains(out.Impls[0].Label, "ledger") {
		t.Errorf("expected ledger impl hit; got %+v", out.Impls)
	}
	if len(out.Features) == 0 || !strings.Contains(out.Features[0].Label, "ledger") {
		t.Errorf("expected ledger feature hit; got %+v", out.Features)
	}
	if len(out.Branches) == 0 || !strings.Contains(out.Branches[0].Label, "ledger") {
		t.Errorf("expected ledger branch hit; got %+v", out.Branches)
	}

	// "billing" should match only the second product.
	resp2 := client.GET("/t/searchteam/search?q=billing", nil)
	resp2.AssertStatus(http.StatusOK)
	var out2 searchResponse
	_ = json.Unmarshal(resp2.Body(), &out2)
	if len(out2.Products) != 1 || out2.Products[0].Label != "billing" {
		t.Errorf("expected single billing hit; got %+v", out2.Products)
	}

	// Empty query returns empty groups.
	resp3 := client.GET("/t/searchteam/search?q=", nil)
	resp3.AssertStatus(http.StatusOK)
	var out3 searchResponse
	_ = json.Unmarshal(resp3.Body(), &out3)
	if len(out3.Products)+len(out3.Impls)+len(out3.Features)+len(out3.Branches) != 0 {
		t.Errorf("expected empty groups for empty query; got %+v", out3)
	}
}

func TestSearch_404ForNonMember(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "search-outsider@test.example"})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "search-owner@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "secretsearch"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/secretsearch/search?q=anything", nil)
	if resp.Status() != http.StatusNotFound {
		t.Fatalf("expected 404 for non-member; got %d", resp.Status())
	}
}
