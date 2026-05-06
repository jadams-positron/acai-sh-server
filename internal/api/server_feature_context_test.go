package api_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

// fcResp is a loose shape for decoding feature-context JSON responses.
// We avoid depending on spec.FeatureContextResponse to keep tests flexible;
// the actual spec type uses anonymous structs which are awkward to decode into.
type fcResp struct {
	Data struct {
		Acids []struct {
			Acid          string   `json:"acid"`
			Deprecated    *bool    `json:"deprecated"`
			Note          *string  `json:"note"`
			RefsCount     int      `json:"refs_count"`
			TestRefsCount int      `json:"test_refs_count"`
			Requirement   string   `json:"requirement"`
			State         fcState  `json:"state"`
			Refs          []fcRef  `json:"refs"`
			ReplacedBy    []string `json:"replaced_by"`
		} `json:"acids"`
		DanglingStates []struct {
			Acid  string  `json:"acid"`
			State fcState `json:"state"`
		} `json:"dangling_states"`
		FeatureName        string    `json:"feature_name"`
		ImplementationId   string    `json:"implementation_id"` //nolint:staticcheck // ST1003: matches generated JSON tag
		ImplementationName string    `json:"implementation_name"`
		ProductName        string    `json:"product_name"`
		SpecSource         fcSource  `json:"spec_source"`
		RefsSource         fcSource  `json:"refs_source"`
		StatesSource       fcSource  `json:"states_source"`
		Summary            fcSummary `json:"summary"`
		Warnings           []string  `json:"warnings"`
	} `json:"data"`
}

type fcState struct {
	Status    *string `json:"status"`
	Comment   *string `json:"comment"`
	UpdatedAt *string `json:"updated_at"`
}

type fcRef struct {
	BranchName string `json:"branch_name"`
	IsTest     bool   `json:"is_test"`
	Path       string `json:"path"`
	RepoUri    string `json:"repo_uri"` //nolint:staticcheck // ST1003: matches generated JSON tag
}

type fcSource struct {
	SourceType         string    `json:"source_type"`
	ImplementationName *string   `json:"implementation_name"`
	BranchNames        *[]string `json:"branch_names"`
}

type fcSummary struct {
	TotalAcids   int            `json:"total_acids"`
	StatusCounts map[string]any `json:"status_counts"`
}

// seedHappyPath seeds a product, impl, branch, spec, refs, and states for use
// across multiple sub-tests. Returns the bearer token.
func seedHappyPath(t *testing.T, app *testfx.App) (plaintext string) {
	t.Helper()
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "acme"})
	_, plaintext = testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "myapp"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "production"})
	branch := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{
		RepoURI: "github.com/test/repo", BranchName: "main",
	})
	testfx.SeedTrackedBranch(t, app.DB, impl, branch)
	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{
		FeatureName: "auth-feature",
		Description: "User authentication",
		Requirements: map[string]any{
			"auth-feature.AUTH.1": map[string]any{
				"requirement": "Validate email format",
			},
			"auth-feature.AUTH.2": map[string]any{
				"requirement": "Hash passwords",
				"deprecated":  true,
			},
		},
	})
	testfx.SeedFeatureBranchRef(t, app.DB, branch, "auth-feature", map[string]any{
		"auth-feature.AUTH.1": []map[string]any{
			{"path": "lib/auth.go:42", "is_test": false},
			{"path": "lib/auth_test.go:10", "is_test": true},
		},
	})
	testfx.SeedFeatureImplState(t, app.DB, impl, "auth-feature", map[string]any{
		"auth-feature.AUTH.1": map[string]any{
			"status":  "completed",
			"comment": "Done in v1.2",
		},
	})
	return plaintext
}

func TestFeatureContext_HappyPath(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	token := seedHappyPath(t, app)

	resp := app.Client().WithBearer(token).GET("/api/v1/feature-context", url.Values{
		"product_name":        {"myapp"},
		"feature_name":        {"auth-feature"},
		"implementation_name": {"production"},
	})
	resp.AssertStatus(http.StatusOK)

	var doc fcResp
	resp.JSON(&doc)

	// Deprecated AUTH.2 is filtered out by default.
	if len(doc.Data.Acids) != 1 {
		t.Fatalf("got %d acids, want 1 (deprecated filtered); body=%s", len(doc.Data.Acids), resp.Body())
	}
	if doc.Data.Acids[0].Acid != "auth-feature.AUTH.1" {
		t.Errorf("acid = %q, want auth-feature.AUTH.1", doc.Data.Acids[0].Acid)
	}
	if doc.Data.Acids[0].RefsCount != 1 || doc.Data.Acids[0].TestRefsCount != 1 {
		t.Errorf("refs=%d test_refs=%d, want 1/1", doc.Data.Acids[0].RefsCount, doc.Data.Acids[0].TestRefsCount)
	}
	if doc.Data.Acids[0].State.Status == nil || *doc.Data.Acids[0].State.Status != "completed" {
		t.Errorf("status = %v, want completed", doc.Data.Acids[0].State.Status)
	}
	if doc.Data.Acids[0].State.Comment == nil || *doc.Data.Acids[0].State.Comment != "Done in v1.2" {
		t.Errorf("comment = %v, want Done in v1.2", doc.Data.Acids[0].State.Comment)
	}
	if doc.Data.FeatureName != "auth-feature" {
		t.Errorf("feature_name = %q, want auth-feature", doc.Data.FeatureName)
	}
	if doc.Data.ProductName != "myapp" {
		t.Errorf("product_name = %q, want myapp", doc.Data.ProductName)
	}
	if doc.Data.ImplementationName != "production" {
		t.Errorf("implementation_name = %q, want production", doc.Data.ImplementationName)
	}
	if doc.Data.SpecSource.SourceType != "local" {
		t.Errorf("spec_source.source_type = %q, want local", doc.Data.SpecSource.SourceType)
	}
	if doc.Data.RefsSource.SourceType != "local" {
		t.Errorf("refs_source.source_type = %q, want local", doc.Data.RefsSource.SourceType)
	}
	if doc.Data.StatesSource.SourceType != "local" {
		t.Errorf("states_source.source_type = %q, want local", doc.Data.StatesSource.SourceType)
	}
	if doc.Data.Summary.TotalAcids != 1 {
		t.Errorf("summary.total_acids = %d, want 1", doc.Data.Summary.TotalAcids)
	}
}

func TestFeatureContext_IncludeDeprecated(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	token := seedHappyPath(t, app)

	resp := app.Client().WithBearer(token).GET("/api/v1/feature-context", url.Values{
		"product_name":        {"myapp"},
		"feature_name":        {"auth-feature"},
		"implementation_name": {"production"},
		"include_deprecated":  {"true"},
	})
	resp.AssertStatus(http.StatusOK)

	var doc fcResp
	resp.JSON(&doc)

	if len(doc.Data.Acids) != 2 {
		t.Fatalf("got %d acids, want 2 (deprecated included); body=%s", len(doc.Data.Acids), resp.Body())
	}

	// Verify AUTH.2 appears and has deprecated flag.
	found := false
	for _, a := range doc.Data.Acids {
		if a.Acid == "auth-feature.AUTH.2" {
			found = true
			if a.Deprecated == nil || !*a.Deprecated {
				t.Errorf("AUTH.2 deprecated flag = %v, want true", a.Deprecated)
			}
		}
	}
	if !found {
		t.Errorf("auth-feature.AUTH.2 not found in acids; body=%s", resp.Body())
	}
}

func TestFeatureContext_StatusFilter(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "filterapp"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "main"})
	branch := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{})
	testfx.SeedTrackedBranch(t, app.DB, impl, branch)

	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{
		FeatureName: "feat",
		Requirements: map[string]any{
			"feat.F.1": map[string]any{"requirement": "req1"},
			"feat.F.2": map[string]any{"requirement": "req2"},
			"feat.F.3": map[string]any{"requirement": "req3"},
		},
	})
	testfx.SeedFeatureImplState(t, app.DB, impl, "feat", map[string]any{
		"feat.F.1": map[string]any{"status": "completed"},
		"feat.F.2": map[string]any{"status": "incomplete"},
		// feat.F.3 has no state → null status
	})

	// Filter to completed only.
	resp := app.Client().WithBearer(token).GET("/api/v1/feature-context", url.Values{
		"product_name":        {"filterapp"},
		"feature_name":        {"feat"},
		"implementation_name": {"main"},
		"statuses":            {"completed"},
	})
	resp.AssertStatus(http.StatusOK)

	var doc fcResp
	resp.JSON(&doc)
	if len(doc.Data.Acids) != 1 {
		t.Fatalf("got %d acids, want 1 (completed only); body=%s", len(doc.Data.Acids), resp.Body())
	}
	if doc.Data.Acids[0].Acid != "feat.F.1" {
		t.Errorf("acid = %q, want feat.F.1", doc.Data.Acids[0].Acid)
	}

	// Filter to null status (no state set).
	resp2 := app.Client().WithBearer(token).GET("/api/v1/feature-context", url.Values{
		"product_name":        {"filterapp"},
		"feature_name":        {"feat"},
		"implementation_name": {"main"},
		"statuses":            {"null"},
	})
	resp2.AssertStatus(http.StatusOK)

	var doc2 fcResp
	resp2.JSON(&doc2)
	if len(doc2.Data.Acids) != 1 {
		t.Fatalf("got %d acids, want 1 (null status only); body=%s", len(doc2.Data.Acids), resp2.Body())
	}
	if doc2.Data.Acids[0].Acid != "feat.F.3" {
		t.Errorf("acid = %q, want feat.F.3", doc2.Data.Acids[0].Acid)
	}
}

func TestFeatureContext_DanglingStates(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "dangapp"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "main"})
	branch := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{})
	testfx.SeedTrackedBranch(t, app.DB, impl, branch)

	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{
		FeatureName:  "feat",
		Requirements: map[string]any{"feat.F.1": map[string]any{"requirement": "req1"}},
	})
	// State for feat.F.GONE doesn't exist in spec — it's dangling.
	testfx.SeedFeatureImplState(t, app.DB, impl, "feat", map[string]any{
		"feat.F.1":    map[string]any{"status": "completed"},
		"feat.F.GONE": map[string]any{"status": "incomplete"},
	})

	// Without include_dangling_states — dangling_states is absent/null.
	resp := app.Client().WithBearer(token).GET("/api/v1/feature-context", url.Values{
		"product_name":        {"dangapp"},
		"feature_name":        {"feat"},
		"implementation_name": {"main"},
	})
	resp.AssertStatus(http.StatusOK)
	var docDefault fcResp
	resp.JSON(&docDefault)
	if docDefault.Data.DanglingStates != nil {
		t.Errorf("dangling_states expected nil by default, got %v", docDefault.Data.DanglingStates)
	}

	// With include_dangling_states=true.
	resp2 := app.Client().WithBearer(token).GET("/api/v1/feature-context", url.Values{
		"product_name":            {"dangapp"},
		"feature_name":            {"feat"},
		"implementation_name":     {"main"},
		"include_dangling_states": {"true"},
	})
	resp2.AssertStatus(http.StatusOK)
	var docDangling fcResp
	resp2.JSON(&docDangling)
	if len(docDangling.Data.DanglingStates) != 1 {
		t.Fatalf("got %d dangling, want 1; body=%s", len(docDangling.Data.DanglingStates), resp2.Body())
	}
	if docDangling.Data.DanglingStates[0].Acid != "feat.F.GONE" {
		t.Errorf("dangling acid = %q, want feat.F.GONE", docDangling.Data.DanglingStates[0].Acid)
	}
}

func TestFeatureContext_ProductNotFound(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})

	resp := app.Client().WithBearer(token).GET("/api/v1/feature-context", url.Values{
		"product_name":        {"nonexistent"},
		"feature_name":        {"feat"},
		"implementation_name": {"main"},
	})
	resp.AssertStatus(http.StatusNotFound)
}

func TestFeatureContext_ImplementationNotFound(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "existingprod"})
	_ = product

	resp := app.Client().WithBearer(token).GET("/api/v1/feature-context", url.Values{
		"product_name":        {"existingprod"},
		"feature_name":        {"feat"},
		"implementation_name": {"nonexistent"},
	})
	resp.AssertStatus(http.StatusNotFound)
}

func TestFeatureContext_NoBearer(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	resp := app.Client().GET("/api/v1/feature-context", url.Values{
		"product_name":        {"myapp"},
		"feature_name":        {"feat"},
		"implementation_name": {"main"},
	})
	resp.AssertStatus(http.StatusUnauthorized)
}

func TestFeatureContext_NoSpec(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "nospecapp"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "main"})
	branch := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{})
	testfx.SeedTrackedBranch(t, app.DB, impl, branch)
	// No spec seeded — no refs seeded either.

	resp := app.Client().WithBearer(token).GET("/api/v1/feature-context", url.Values{
		"product_name":        {"nospecapp"},
		"feature_name":        {"feat"},
		"implementation_name": {"main"},
	})
	resp.AssertStatus(http.StatusOK)

	var doc fcResp
	resp.JSON(&doc)
	if len(doc.Data.Acids) != 0 {
		t.Errorf("got %d acids, want 0 (no spec)", len(doc.Data.Acids))
	}
	if doc.Data.SpecSource.SourceType != "none" {
		t.Errorf("spec_source.source_type = %q, want none", doc.Data.SpecSource.SourceType)
	}
}
