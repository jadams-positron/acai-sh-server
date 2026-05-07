package handlers_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/services"
	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

// makeImplSlug builds a slug from a SeededImplementation, matching the
// format used by ImplementationCard.ImplementationSlug (name + "-" + uuid-no-dashes).
func makeImplSlug(impl *testfx.SeededImplementation) string {
	noDashes := strings.ReplaceAll(impl.ID, "-", "")
	return impl.Name + "-" + noDashes
}

// roundTripSlug verifies that ParseImplSlug(makeImplSlug(impl)) == impl.ID.
func roundTripSlug(impl *testfx.SeededImplementation) bool {
	slug := makeImplSlug(impl)
	got := services.ParseImplSlug(slug)
	return got == impl.ID
}

func TestParseImplSlug(t *testing.T) {
	tests := []struct {
		name   string
		slug   string
		wantID string
	}{
		{
			name:   "valid slug",
			slug:   "my-impl-01234567890123456789012345678901",
			wantID: "01234567-8901-2345-6789-012345678901",
		},
		{
			name:   "impl name with dashes",
			slug:   "my-cool-impl-0a1b2c3d0a1b2c3d0a1b2c3d0a1b2c3d",
			wantID: "0a1b2c3d-0a1b-2c3d-0a1b-2c3d0a1b2c3d",
		},
		{
			name:   "too short suffix",
			slug:   "impl-abc123",
			wantID: "",
		},
		{
			name:   "no dash",
			slug:   "nodash0123456789012345678901234567890",
			wantID: "",
		},
		{
			name:   "empty string",
			slug:   "",
			wantID: "",
		},
		{
			name:   "suffix not hex",
			slug:   "impl-zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
			wantID: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := services.ParseImplSlug(tc.slug)
			if got != tc.wantID {
				t.Errorf("ParseImplSlug(%q) = %q, want %q", tc.slug, got, tc.wantID)
			}
		})
	}
}

func TestImplFeatureShow_RendersACIDTable(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "acid-member@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "acid-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "acid-product"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "acid-impl"})
	branch := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{RepoURI: "github.com/test/acid-repo"})
	testfx.SeedTrackedBranch(t, app.DB, impl, branch)

	featureName := "acid-feature"
	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{
		FeatureName: featureName,
		Requirements: map[string]any{
			"AC-001": map[string]any{"requirement": "The system shall do X"},
			"AC-002": map[string]any{"requirement": "The system shall do Y"},
		},
	})
	testfx.SeedFeatureImplState(t, app.DB, impl, featureName, map[string]any{
		"AC-001": map[string]any{"status": "completed", "comment": "done!"},
		"AC-002": map[string]any{"status": "blocked"},
	})
	testfx.SeedFeatureBranchRef(t, app.DB, branch, featureName, map[string]any{
		"AC-001": []map[string]any{
			{"path": "internal/foo.go", "is_test": false},
			{"path": "internal/foo_test.go", "is_test": true},
		},
	})

	if !roundTripSlug(impl) {
		t.Fatalf("slug round-trip failed: got %q for impl ID %q", makeImplSlug(impl), impl.ID)
	}

	slug := makeImplSlug(impl)
	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/acid-team/i/"+slug+"/f/"+featureName, nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	if !strings.Contains(body, "AC-001") {
		t.Errorf("expected AC-001 in body; got %.500s", body)
	}
	if !strings.Contains(body, "AC-002") {
		t.Errorf("expected AC-002 in body; got %.500s", body)
	}
	if !strings.Contains(body, "The system shall do X") {
		t.Errorf("expected requirement text in body; got %.500s", body)
	}
	if !strings.Contains(body, "completed") {
		t.Errorf("expected 'completed' status pill in body; got %.500s", body)
	}
	if !strings.Contains(body, "blocked") {
		t.Errorf("expected 'blocked' status pill in body; got %.500s", body)
	}
	if !strings.Contains(body, "done!") {
		t.Errorf("expected comment text in body; got %.500s", body)
	}
	// Refs count: AC-001 has 1 src + 1 test ref.
	if !strings.Contains(body, "1 (+1 tests)") {
		t.Errorf("expected refs count '1 (+1 tests)' in body; got %.500s", body)
	}
	// The breadcrumb should include the team and impl name.
	if !strings.Contains(body, "acid-team") {
		t.Errorf("expected team name in breadcrumb; got %.500s", body)
	}
}

func TestImplFeatureSetStatus_HappyPath_Redirects(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "set-status@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "setstatus-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "p"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "i"})
	branch := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{RepoURI: "github.com/test/r"})
	testfx.SeedTrackedBranch(t, app.DB, impl, branch)
	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{
		FeatureName: "f",
		Requirements: map[string]any{
			"AC-1": map[string]any{"requirement": "first"},
		},
	})

	slug := makeImplSlug(impl)
	client := testfx.LoggedInClient(t, app, user)
	csrfToken, postClient := getCSRFToken(t, client, "/t/setstatus-team/i/"+slug+"/f/f")

	statusURL := "/t/setstatus-team/i/" + slug + "/f/f/acid/AC-1/status"
	resp := postClient.POSTForm(statusURL, url.Values{
		"status":             {"completed"},
		"gorilla.csrf.Token": {csrfToken},
	})
	if resp.Status() != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect; got %d, body=%.500s", resp.Status(), resp.Body())
	}
	if loc := resp.Header("Location"); loc != "/t/setstatus-team/i/"+slug+"/f/f" {
		t.Errorf("Location = %q; want drilldown URL", loc)
	}

	// Re-fetch the page; AC-1 should now appear in completed-status form.
	getResp := client.GET("/t/setstatus-team/i/"+slug+"/f/f", nil)
	getResp.AssertStatus(http.StatusOK)
	body := string(getResp.Body())
	// The new select-based status renders <option value="completed" selected>.
	if !strings.Contains(body, `value="completed" selected`) {
		t.Errorf("expected completed option to be selected; got %.800s", body)
	}
}

func TestImplFeatureSetStatus_InvalidStatus_422(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "bad-status@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "badstatus-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "p"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "i"})

	slug := makeImplSlug(impl)
	client := testfx.LoggedInClient(t, app, user)
	csrfToken, postClient := getCSRFToken(t, client, "/t/badstatus-team/i/"+slug+"/f/f")

	resp := postClient.POSTForm("/t/badstatus-team/i/"+slug+"/f/f/acid/AC-1/status", url.Values{
		"status":             {"banana"},
		"gorilla.csrf.Token": {csrfToken},
	})
	if resp.Status() != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for bad status; got %d", resp.Status())
	}
}

func TestImplFeatureShow_NoSpec_EmptyState(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "acid-nospec@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "acid-nospec-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "nospec-product"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "nospec-impl"})
	// No branch, spec, or states seeded.

	slug := makeImplSlug(impl)
	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/acid-nospec-team/i/"+slug+"/f/some-feature", nil)
	resp.AssertStatus(http.StatusOK)

	body := string(resp.Body())
	if !strings.Contains(body, "No spec for") {
		t.Errorf("expected empty-state copy 'No spec for' in body; got %.500s", body)
	}
}

func TestImplFeatureShow_404ForNonMember(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	nonMember := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "acid-nonmember@test.example"})
	owner := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "acid-owner@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "acid-secret-team"})
	testfx.SeedUserTeamRole(t, app.DB, owner, team, "owner")

	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "secret-product"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "secret-impl"})
	slug := makeImplSlug(impl)

	client := testfx.LoggedInClient(t, app, nonMember)
	resp := client.GET("/t/acid-secret-team/i/"+slug+"/f/some-feature", nil)
	if resp.Status() != http.StatusNotFound {
		t.Fatalf("expected 404 for non-member; got %d, body=%.500s", resp.Status(), resp.Body())
	}
}

func TestImplFeatureShow_RedirectsAnon(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "acid-anon-team"})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "anon-product"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "anon-impl"})
	slug := makeImplSlug(impl)

	client := app.Client()
	resp := client.GET("/t/acid-anon-team/i/"+slug+"/f/some-feature", nil)
	if resp.Status() != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect for anon; got %d", resp.Status())
	}
	if loc := resp.Header("Location"); loc != "/users/log-in" {
		t.Errorf("Location = %q, want /users/log-in", loc)
	}
}

func TestImplFeatureShow_404ForBadSlug(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{Email: "acid-badslug@test.example"})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "acid-badslug-team"})
	testfx.SeedUserTeamRole(t, app.DB, user, team, "owner")

	client := testfx.LoggedInClient(t, app, user)
	resp := client.GET("/t/acid-badslug-team/i/not-a-valid-slug/f/some-feature", nil)
	if resp.Status() != http.StatusNotFound {
		t.Fatalf("expected 404 for bad slug; got %d, body=%.500s", resp.Status(), resp.Body())
	}
}
