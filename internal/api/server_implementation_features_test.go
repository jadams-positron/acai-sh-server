package api_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

// ifResp is a loose shape for decoding implementation-features JSON responses.
type ifResp struct {
	Data struct {
		Features []struct {
			FeatureName        string  `json:"feature_name"`
			Description        *string `json:"description"`
			SpecLastSeenCommit *string `json:"spec_last_seen_commit"`
			HasLocalSpec       bool    `json:"has_local_spec"`
			HasLocalStates     bool    `json:"has_local_states"`
			RefsInherited      bool    `json:"refs_inherited"`
			StatesInherited    bool    `json:"states_inherited"`
			RefsCount          int     `json:"refs_count"`
			TestRefsCount      int     `json:"test_refs_count"`
			TotalCount         int     `json:"total_count"`
			CompletedCount     int     `json:"completed_count"`
		} `json:"features"`
		ImplementationId   string `json:"implementation_id"`   //nolint:staticcheck // ST1003: matches generated JSON tag
		ImplementationName string `json:"implementation_name"` //nolint:staticcheck // ST1003: matches generated JSON tag
		ProductName        string `json:"product_name"`
	} `json:"data"`
}

func TestImplementationFeatures_HappyPath(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "acme"})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "myapp"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "production"})
	branch := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{
		RepoURI:        "github.com/test/repo",
		BranchName:     "main",
		LastSeenCommit: "deadbeef",
	})
	testfx.SeedTrackedBranch(t, app.DB, impl, branch)

	// feature1: spec with 2 reqs, states for 1 completed, refs with 2 refs (1 test).
	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{
		FeatureName: "feature1",
		Description: "Feature one description",
		Requirements: map[string]any{
			"feature1.F.1": map[string]any{"requirement": "req one"},
			"feature1.F.2": map[string]any{"requirement": "req two"},
		},
	})
	testfx.SeedFeatureImplState(t, app.DB, impl, "feature1", map[string]any{
		"feature1.F.1": map[string]any{"status": "completed"},
		"feature1.F.2": map[string]any{"status": "incomplete"},
	})
	testfx.SeedFeatureBranchRef(t, app.DB, branch, "feature1", map[string]any{
		"feature1.F.1": []map[string]any{
			{"path": "lib/feature1.go:10", "is_test": false},
			{"path": "lib/feature1_test.go:5", "is_test": true},
		},
	})

	// feature2: spec with 1 req, no states, no refs.
	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{
		FeatureName: "feature2",
		Description: "Feature two description",
		Requirements: map[string]any{
			"feature2.F.1": map[string]any{"requirement": "req one"},
		},
	})

	resp := app.Client().WithBearer(token).GET("/api/v1/implementation-features", url.Values{
		"product_name":        {"myapp"},
		"implementation_name": {"production"},
	})
	resp.AssertStatus(http.StatusOK)

	var doc ifResp
	resp.JSON(&doc)

	if len(doc.Data.Features) != 2 {
		t.Fatalf("got %d features, want 2; body=%s", len(doc.Data.Features), resp.Body())
	}

	// features must be sorted alphabetically.
	if doc.Data.Features[0].FeatureName != "feature1" {
		t.Errorf("features[0].feature_name = %q, want feature1", doc.Data.Features[0].FeatureName)
	}
	if doc.Data.Features[1].FeatureName != "feature2" {
		t.Errorf("features[1].feature_name = %q, want feature2", doc.Data.Features[1].FeatureName)
	}

	f1 := doc.Data.Features[0]
	if f1.TotalCount != 2 {
		t.Errorf("feature1.total_count = %d, want 2", f1.TotalCount)
	}
	if f1.CompletedCount != 1 {
		t.Errorf("feature1.completed_count = %d, want 1", f1.CompletedCount)
	}
	if f1.RefsCount != 1 {
		t.Errorf("feature1.refs_count = %d, want 1", f1.RefsCount)
	}
	if f1.TestRefsCount != 1 {
		t.Errorf("feature1.test_refs_count = %d, want 1", f1.TestRefsCount)
	}
	if !f1.HasLocalSpec {
		t.Errorf("feature1.has_local_spec = false, want true")
	}
	if !f1.HasLocalStates {
		t.Errorf("feature1.has_local_states = false, want true")
	}
	if f1.RefsInherited {
		t.Errorf("feature1.refs_inherited = true, want false (P2b-3)")
	}
	if f1.StatesInherited {
		t.Errorf("feature1.states_inherited = true, want false (P2b-3)")
	}
	if f1.Description == nil || *f1.Description != "Feature one description" {
		t.Errorf("feature1.description = %v, want Feature one description", f1.Description)
	}

	f2 := doc.Data.Features[1]
	if f2.TotalCount != 1 {
		t.Errorf("feature2.total_count = %d, want 1", f2.TotalCount)
	}
	if f2.CompletedCount != 0 {
		t.Errorf("feature2.completed_count = %d, want 0", f2.CompletedCount)
	}
	if f2.RefsCount != 0 {
		t.Errorf("feature2.refs_count = %d, want 0", f2.RefsCount)
	}
	if !f2.HasLocalSpec {
		t.Errorf("feature2.has_local_spec = false, want true")
	}
	if f2.HasLocalStates {
		t.Errorf("feature2.has_local_states = true, want false")
	}

	// Top-level metadata.
	if doc.Data.ProductName != "myapp" {
		t.Errorf("product_name = %q, want myapp", doc.Data.ProductName)
	}
	if doc.Data.ImplementationName != "production" {
		t.Errorf("implementation_name = %q, want production", doc.Data.ImplementationName)
	}
	if doc.Data.ImplementationId == "" {
		t.Errorf("implementation_id is empty")
	}
}

func TestImplementationFeatures_StatusFilter_Completed(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "filterapp"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "main"})
	branch := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{})
	testfx.SeedTrackedBranch(t, app.DB, impl, branch)

	// feature1: has a completed ACID.
	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{
		FeatureName:  "feature1",
		Requirements: map[string]any{"feature1.F.1": map[string]any{"requirement": "r1"}},
	})
	testfx.SeedFeatureImplState(t, app.DB, impl, "feature1", map[string]any{
		"feature1.F.1": map[string]any{"status": "completed"},
	})

	// feature2: all incomplete — should NOT appear with statuses=completed.
	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{
		FeatureName:  "feature2",
		Requirements: map[string]any{"feature2.F.1": map[string]any{"requirement": "r2"}},
	})
	testfx.SeedFeatureImplState(t, app.DB, impl, "feature2", map[string]any{
		"feature2.F.1": map[string]any{"status": "incomplete"},
	})

	resp := app.Client().WithBearer(token).GET("/api/v1/implementation-features", url.Values{
		"product_name":        {product.Name},
		"implementation_name": {"main"},
		"statuses":            {"completed"},
	})
	resp.AssertStatus(http.StatusOK)

	var doc ifResp
	resp.JSON(&doc)

	if len(doc.Data.Features) != 1 {
		t.Fatalf("got %d features, want 1 (completed only); body=%s", len(doc.Data.Features), resp.Body())
	}
	if doc.Data.Features[0].FeatureName != "feature1" {
		t.Errorf("feature = %q, want feature1", doc.Data.Features[0].FeatureName)
	}
}

func TestImplementationFeatures_StatusFilter_NullSentinel(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "nullapp"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "main"})
	branch := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{})
	testfx.SeedTrackedBranch(t, app.DB, impl, branch)

	// feature1: has a null-status ACID (no state entry for this ACID).
	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{
		FeatureName: "feature1",
		Requirements: map[string]any{
			"feature1.F.1": map[string]any{"requirement": "r1"},
			"feature1.F.2": map[string]any{"requirement": "r2"},
		},
	})
	// State only for F.1 — F.2 has no state, so null status.
	testfx.SeedFeatureImplState(t, app.DB, impl, "feature1", map[string]any{
		"feature1.F.1": map[string]any{"status": "completed"},
	})

	// feature2: all completed — no null-status ACIDs.
	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{
		FeatureName:  "feature2",
		Requirements: map[string]any{"feature2.F.1": map[string]any{"requirement": "r3"}},
	})
	testfx.SeedFeatureImplState(t, app.DB, impl, "feature2", map[string]any{
		"feature2.F.1": map[string]any{"status": "completed"},
	})

	resp := app.Client().WithBearer(token).GET("/api/v1/implementation-features", url.Values{
		"product_name":        {product.Name},
		"implementation_name": {"main"},
		"statuses":            {"null"},
	})
	resp.AssertStatus(http.StatusOK)

	var doc ifResp
	resp.JSON(&doc)

	// Only feature1 has a null-status ACID (feature1.F.2 has no state).
	if len(doc.Data.Features) != 1 {
		t.Fatalf("got %d features, want 1 (null sentinel); body=%s", len(doc.Data.Features), resp.Body())
	}
	if doc.Data.Features[0].FeatureName != "feature1" {
		t.Errorf("feature = %q, want feature1", doc.Data.Features[0].FeatureName)
	}
}

func TestImplementationFeatures_ChangedSinceCommit(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "commitapp"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "main"})
	// Use a branch with a base commit; we'll override LastSeenCommit per spec.
	branch := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{
		LastSeenCommit: "commit-a",
	})
	testfx.SeedTrackedBranch(t, app.DB, impl, branch)

	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{
		FeatureName:    "feature1",
		LastSeenCommit: "commit-a", // matches the filter
		Requirements:   map[string]any{"feature1.F.1": map[string]any{"requirement": "r1"}},
	})
	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{
		FeatureName:    "feature2",
		LastSeenCommit: "commit-b", // different commit, should be excluded
		Requirements:   map[string]any{"feature2.F.1": map[string]any{"requirement": "r2"}},
	})

	resp := app.Client().WithBearer(token).GET("/api/v1/implementation-features", url.Values{
		"product_name":         {product.Name},
		"implementation_name":  {"main"},
		"changed_since_commit": {"commit-a"},
	})
	resp.AssertStatus(http.StatusOK)

	var doc ifResp
	resp.JSON(&doc)

	if len(doc.Data.Features) != 1 {
		t.Fatalf("got %d features, want 1 (commit-a filter); body=%s", len(doc.Data.Features), resp.Body())
	}
	if doc.Data.Features[0].FeatureName != "feature1" {
		t.Errorf("feature = %q, want feature1", doc.Data.Features[0].FeatureName)
	}
}

func TestImplementationFeatures_ProductNotFound_404(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})

	resp := app.Client().WithBearer(token).GET("/api/v1/implementation-features", url.Values{
		"product_name":        {"nonexistent"},
		"implementation_name": {"main"},
	})
	resp.AssertStatus(http.StatusNotFound)
}

func TestImplementationFeatures_ImplementationNotFound_404(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "existingprod"})
	_ = product

	resp := app.Client().WithBearer(token).GET("/api/v1/implementation-features", url.Values{
		"product_name":        {"existingprod"},
		"implementation_name": {"nonexistent"},
	})
	resp.AssertStatus(http.StatusNotFound)
}

func TestImplementationFeatures_NoBearer_401(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	resp := app.Client().GET("/api/v1/implementation-features", url.Values{
		"product_name":        {"myapp"},
		"implementation_name": {"main"},
	})
	resp.AssertStatus(http.StatusUnauthorized)
}

func TestImplementationFeatures_StatesWithoutSpec(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "danglingapp"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "main"})
	branch := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{})
	testfx.SeedTrackedBranch(t, app.DB, impl, branch)

	// States for a feature that has NO spec — "dangling state" feature.
	testfx.SeedFeatureImplState(t, app.DB, impl, "orphan-feature", map[string]any{
		"orphan-feature.F.1": map[string]any{"status": "completed"},
	})

	resp := app.Client().WithBearer(token).GET("/api/v1/implementation-features", url.Values{
		"product_name":        {product.Name},
		"implementation_name": {"main"},
	})
	resp.AssertStatus(http.StatusOK)

	var doc ifResp
	resp.JSON(&doc)

	if len(doc.Data.Features) != 1 {
		t.Fatalf("got %d features, want 1 (dangling state); body=%s", len(doc.Data.Features), resp.Body())
	}
	f := doc.Data.Features[0]
	if f.FeatureName != "orphan-feature" {
		t.Errorf("feature_name = %q, want orphan-feature", f.FeatureName)
	}
	if f.HasLocalSpec {
		t.Errorf("has_local_spec = true, want false (no spec seeded)")
	}
	if !f.HasLocalStates {
		t.Errorf("has_local_states = false, want true")
	}
	if f.TotalCount != 0 {
		t.Errorf("total_count = %d, want 0 (no spec)", f.TotalCount)
	}
	if f.CompletedCount != 1 {
		t.Errorf("completed_count = %d, want 1", f.CompletedCount)
	}
	if f.Description != nil {
		t.Errorf("description = %v, want nil (no spec)", f.Description)
	}
	if f.SpecLastSeenCommit != nil {
		t.Errorf("spec_last_seen_commit = %v, want nil (no spec)", f.SpecLastSeenCommit)
	}
}

func TestImplementationFeatures_AcceptedCountsAsCompleted(t *testing.T) {
	app := testfx.NewApp(t, testfx.NewAppOpts{})

	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{})
	_, token := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "acceptapp"})
	impl := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "main"})
	branch := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{})
	testfx.SeedTrackedBranch(t, app.DB, impl, branch)

	testfx.SeedSpec(t, app.DB, product, branch, testfx.SeedSpecOpts{
		FeatureName: "feat",
		Requirements: map[string]any{
			"feat.F.1": map[string]any{"requirement": "r1"},
			"feat.F.2": map[string]any{"requirement": "r2"},
			"feat.F.3": map[string]any{"requirement": "r3"},
		},
	})
	// 1 accepted + 1 completed = 2 completed_count; 1 incomplete = 0.
	testfx.SeedFeatureImplState(t, app.DB, impl, "feat", map[string]any{
		"feat.F.1": map[string]any{"status": "accepted"},
		"feat.F.2": map[string]any{"status": "completed"},
		"feat.F.3": map[string]any{"status": "incomplete"},
	})

	resp := app.Client().WithBearer(token).GET("/api/v1/implementation-features", url.Values{
		"product_name":        {product.Name},
		"implementation_name": {"main"},
	})
	resp.AssertStatus(http.StatusOK)

	var doc ifResp
	resp.JSON(&doc)

	if len(doc.Data.Features) != 1 {
		t.Fatalf("got %d features, want 1; body=%s", len(doc.Data.Features), resp.Body())
	}
	if doc.Data.Features[0].CompletedCount != 2 {
		t.Errorf("completed_count = %d, want 2 (accepted+completed)", doc.Data.Features[0].CompletedCount)
	}
}
