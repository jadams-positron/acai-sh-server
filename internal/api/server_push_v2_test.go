package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

// readProductIDByName returns the product ID for (teamID, name) or "" if absent.
func readProductIDByName(t *testing.T, fx *pushFixture, name string) string {
	t.Helper()
	var id string
	err := fx.app.DB.Read.QueryRowContext(context.Background(),
		"SELECT id FROM products WHERE team_id = ? AND name = ?",
		fx.team.ID, name).Scan(&id)
	if err != nil {
		return ""
	}
	return id
}

// readImplByName returns (id, parentImplementationID) for (productID, name).
func readImplByName(t *testing.T, fx *pushFixture, productID, name string) (id string, parentID *string) {
	t.Helper()
	err := fx.app.DB.Read.QueryRowContext(context.Background(),
		"SELECT id, parent_implementation_id FROM implementations WHERE product_id = ? AND name = ?",
		productID, name).Scan(&id, &parentID)
	if err != nil {
		t.Fatalf("readImplByName(%q): %v", name, err)
	}
	return id, parentID
}

// TestPush_AutoCreatesChildImpl_FromParent verifies that a missing target impl is
// created as a child of the given parent impl.
func TestPush_AutoCreatesChildImpl_FromParent(t *testing.T) {
	fx := setupPush(t)
	// fx already has product "myproduct" and impl "production" seeded.

	parentImplName := "production"
	targetImplName := "feature-branch"
	body := pushBody{
		BranchName:     "feat-x",
		CommitHash:     "abc1234",
		RepoURI:        "github.com/test/repo",
		ProductName:    new("myproduct"),
		TargetImplName: new(targetImplName),
		References: &refsPayload{
			Data: map[string][]codeRefPayload{
				"auth-feature.AUTH.1": {{Path: "lib/auth.go:42"}},
			},
		},
	}
	// Add parent_impl_name to the raw body via a wrapper that includes it.
	type pushBodyWithParent struct {
		pushBody
		ParentImplName *string `json:"parent_impl_name,omitempty"`
	}
	fullBody := pushBodyWithParent{
		pushBody:       body,
		ParentImplName: new(parentImplName),
	}

	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", fullBody)
	resp.AssertStatus(http.StatusOK)

	var doc pushResp
	resp.JSON(&doc)
	if doc.Data.ImplementationName == nil || *doc.Data.ImplementationName != targetImplName {
		t.Errorf("implementation_name = %v, want %q", doc.Data.ImplementationName, targetImplName)
	}

	// Verify the new child impl was created in DB with correct parent.
	productID := readProductIDByName(t, fx, "myproduct")
	if productID == "" {
		t.Fatal("product not found in DB")
	}
	childID, childParentID := readImplByName(t, fx, productID, targetImplName)
	if childID == "" {
		t.Fatal("child impl not created in DB")
	}
	if childParentID == nil || *childParentID != fx.impl.ID {
		t.Errorf("parent_implementation_id = %v, want %q", childParentID, fx.impl.ID)
	}
}

// TestPush_NoTargetImpl_NoParent_422 verifies 422 when impl is missing and
// no parent_impl_name is provided.
func TestPush_NoTargetImpl_NoParent_422(t *testing.T) {
	fx := setupPush(t)

	body := pushBody{
		BranchName:     "main",
		CommitHash:     "abc1234",
		RepoURI:        "github.com/test/repo",
		ProductName:    new("myproduct"),
		TargetImplName: new("missing-impl"),
		References: &refsPayload{
			Data: map[string][]codeRefPayload{
				"auth-feature.AUTH.1": {{Path: "lib/auth.go:1"}},
			},
		},
		// No parent_impl_name → must fail with 422.
	}

	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusUnprocessableEntity)
}

// TestPush_TargetMissing_ParentMissing_422 verifies 422 when both target and
// parent impl are absent.
func TestPush_TargetMissing_ParentMissing_422(t *testing.T) {
	fx := setupPush(t)

	type pushBodyWithParent struct {
		pushBody
		ParentImplName *string `json:"parent_impl_name,omitempty"`
	}
	fullBody := pushBodyWithParent{
		pushBody: pushBody{
			BranchName:     "main",
			CommitHash:     "abc1234",
			RepoURI:        "github.com/test/repo",
			ProductName:    new("myproduct"),
			TargetImplName: new("missing-impl"),
			References: &refsPayload{
				Data: map[string][]codeRefPayload{
					"auth-feature.AUTH.1": {{Path: "lib/auth.go:1"}},
				},
			},
		},
		ParentImplName: new("also-missing"),
	}

	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", fullBody)
	resp.AssertStatus(http.StatusUnprocessableEntity)
}

// TestPush_InvalidCommitHash_422 verifies that a non-hex commit_hash returns 422.
func TestPush_InvalidCommitHash_422(t *testing.T) {
	fx := setupPush(t)

	body := pushBody{
		BranchName: "main",
		CommitHash: "not-hex-value",
		RepoURI:    "github.com/test/repo",
		Specs: []pushSpec{
			{
				Feature:      pushFeature{Name: "auth-feature", Product: "myproduct"},
				Meta:         pushMeta{Path: "features/auth.yaml", LastSeenCommit: "abc1234"},
				Requirements: map[string]pushReqDf{},
			},
		},
	}

	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusUnprocessableEntity)
}

// TestPush_ShortValidCommitHash_OK verifies that a 7-character hex short SHA succeeds.
func TestPush_ShortValidCommitHash_OK(t *testing.T) {
	fx := setupPush(t)

	body := pushBody{
		BranchName: "main",
		CommitHash: "abc1234", // 7 hex chars — minimum valid short SHA
		RepoURI:    "github.com/test/repo",
		Specs: []pushSpec{
			{
				Feature:      pushFeature{Name: "auth-feature", Product: "myproduct"},
				Meta:         pushMeta{Path: "features/auth.yaml", LastSeenCommit: "abc1234"},
				Requirements: map[string]pushReqDf{},
			},
		},
	}

	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusOK)
}

// TestPush_AutoCreatesProduct_IdempotentDoublePush verifies that pushing the same
// product name twice is idempotent (the product is not duplicated).
func TestPush_AutoCreatesProduct_IdempotentDoublePush(t *testing.T) {
	fx := setupPush(t)

	// Push a spec referencing a NEW product name (not the fixture's "myproduct").
	spec1 := pushBody{
		BranchName: "main",
		CommitHash: "abc1234",
		RepoURI:    "github.com/test/repo",
		Specs: []pushSpec{
			{
				Feature:      pushFeature{Name: "feat-a", Product: "autoprod"},
				Meta:         pushMeta{Path: "features/a.yaml", LastSeenCommit: "abc1234"},
				Requirements: map[string]pushReqDf{},
			},
		},
	}

	resp1 := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", spec1)
	resp1.AssertStatus(http.StatusOK)

	resp2 := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", spec1)
	resp2.AssertStatus(http.StatusOK)

	// Only one product row should exist for "autoprod" under this team.
	var count int
	err := fx.app.DB.Read.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM products WHERE team_id = ? AND name = ?",
		fx.team.ID, "autoprod").Scan(&count)
	if err != nil {
		t.Fatalf("DB check: %v", err)
	}
	if count != 1 {
		t.Errorf("DB product count = %d, want 1 (idempotent)", count)
	}
}

// ---------------------------------------------------------------------------
// Inference from tracked_branches
// ---------------------------------------------------------------------------

// TestPush_RefsOnly_InferTargetImpl_OneTracking verifies that when exactly one
// impl tracks the pushed branch, the server infers it without requiring
// product_name or target_impl_name in the request.
func TestPush_RefsOnly_InferTargetImpl_OneTracking(t *testing.T) {
	fx := setupPush(t)

	// Seed a branch and link the fixture impl to it.
	branch := testfx.SeedBranch(t, fx.app.DB, fx.team, testfx.SeedBranchOpts{
		RepoURI:    "github.com/infer/repo",
		BranchName: "main",
	})
	testfx.SeedTrackedBranch(t, fx.app.DB, fx.impl, branch)

	// Push refs-only — no product_name, no target_impl_name.
	body := pushBody{
		BranchName: "main",
		CommitHash: "abc1234",
		RepoURI:    "github.com/infer/repo",
		References: &refsPayload{
			Data: map[string][]codeRefPayload{
				"auth-feature.AUTH.1": {{Path: "lib/auth.go:42"}},
			},
		},
	}

	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusOK)

	var doc pushResp
	resp.JSON(&doc)

	// Inference should have resolved to the fixture's "production" impl.
	if doc.Data.ImplementationID == nil {
		t.Fatal("implementation_id should be set when exactly one impl tracks the branch")
	}
	if doc.Data.ImplementationName == nil || *doc.Data.ImplementationName != "production" {
		t.Errorf("implementation_name = %v, want production", doc.Data.ImplementationName)
	}
	if doc.Data.ProductName == nil || *doc.Data.ProductName != "myproduct" {
		t.Errorf("product_name = %v, want myproduct", doc.Data.ProductName)
	}

	// feature_branch_refs row should be written.
	refs := readFeatureRefs(t, fx.app.DB, branch.ID, "auth-feature")
	if len(refs) == 0 {
		t.Error("feature_branch_refs should have refs for auth-feature")
	}
}

// TestPush_RefsOnly_InferTargetImpl_NoTracking verifies that when no impl
// tracks the pushed branch, the server returns 200 with nil impl/product and
// still writes feature_branch_refs.
func TestPush_RefsOnly_InferTargetImpl_NoTracking(t *testing.T) {
	fx := setupPush(t)

	// No tracked_branches seeded for this repo/branch.
	body := pushBody{
		BranchName: "orphan-branch",
		CommitHash: "abc1234",
		RepoURI:    "github.com/infer/repo",
		References: &refsPayload{
			Data: map[string][]codeRefPayload{
				"auth-feature.AUTH.1": {{Path: "lib/auth.go:1"}},
			},
		},
	}

	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusOK)

	var doc pushResp
	resp.JSON(&doc)

	if doc.Data.ImplementationID != nil {
		t.Errorf("implementation_id = %v, want nil (0 tracking impls)", doc.Data.ImplementationID)
	}
	if doc.Data.ProductName != nil {
		t.Errorf("product_name = %v, want nil (0 tracking impls)", doc.Data.ProductName)
	}

	// Refs must still be persisted.
	branchID := readBranchID(t, fx.app.DB, fx.team.ID, "github.com/infer/repo", "orphan-branch")
	refs := readFeatureRefs(t, fx.app.DB, branchID, "auth-feature")
	if len(refs) == 0 {
		t.Error("feature_branch_refs should be written even when no impl tracks the branch")
	}
}

// TestPush_RefsOnly_InferTargetImpl_MultiTracking_NoTarget_422 verifies that
// when more than one impl tracks the branch and target_impl_name is absent the
// server returns 422 with an informative message.
func TestPush_RefsOnly_InferTargetImpl_MultiTracking_NoTarget_422(t *testing.T) {
	fx := setupPush(t)

	// Seed a second product + impl so two impls track the same branch.
	prod2 := testfx.SeedProduct(t, fx.app.DB, fx.team, testfx.SeedProductOpts{Name: "otherproduct"})
	impl2 := testfx.SeedImplementation(t, fx.app.DB, prod2, testfx.SeedImplementationOpts{Name: "staging"})

	branch := testfx.SeedBranch(t, fx.app.DB, fx.team, testfx.SeedBranchOpts{
		RepoURI:    "github.com/multi/repo",
		BranchName: "main",
	})
	testfx.SeedTrackedBranch(t, fx.app.DB, fx.impl, branch)
	testfx.SeedTrackedBranch(t, fx.app.DB, impl2, branch)

	// Push without target_impl_name → must 422.
	body := pushBody{
		BranchName: "main",
		CommitHash: "abc1234",
		RepoURI:    "github.com/multi/repo",
		References: &refsPayload{
			Data: map[string][]codeRefPayload{
				"auth-feature.AUTH.1": {{Path: "lib/auth.go:1"}},
			},
		},
	}

	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusUnprocessableEntity)

	// Verify the error body mentions "multiple implementations".
	raw := string(resp.Body())
	if !strings.Contains(raw, "multiple implementations") {
		t.Errorf("error body should mention 'multiple implementations', got: %s", raw)
	}
}

// TestPush_RefsOnly_InferTargetImpl_MultiTracking_WithTarget verifies that when
// multiple impls track the same branch, providing target_impl_name disambiguates.
func TestPush_RefsOnly_InferTargetImpl_MultiTracking_WithTarget(t *testing.T) {
	fx := setupPush(t)

	// Seed a second product + impl.
	prod2 := testfx.SeedProduct(t, fx.app.DB, fx.team, testfx.SeedProductOpts{Name: "otherproduct"})
	impl2 := testfx.SeedImplementation(t, fx.app.DB, prod2, testfx.SeedImplementationOpts{Name: "staging"})

	branch := testfx.SeedBranch(t, fx.app.DB, fx.team, testfx.SeedBranchOpts{
		RepoURI:    "github.com/multi2/repo",
		BranchName: "main",
	})
	testfx.SeedTrackedBranch(t, fx.app.DB, fx.impl, branch) // "production"
	testfx.SeedTrackedBranch(t, fx.app.DB, impl2, branch)   // "staging"

	// Push with target_impl_name="staging" but no product_name.
	stagingName := "staging"
	body := pushBody{
		BranchName:     "main",
		CommitHash:     "abc1234",
		RepoURI:        "github.com/multi2/repo",
		TargetImplName: &stagingName,
		// No product_name → inference path with disambiguation.
		References: &refsPayload{
			Data: map[string][]codeRefPayload{
				"auth-feature.AUTH.1": {{Path: "lib/staging.go:7"}},
			},
		},
	}

	resp := fx.app.Client().WithBearer(fx.plaintext).POSTJSON("/api/v1/push", body)
	resp.AssertStatus(http.StatusOK)

	var doc pushResp
	resp.JSON(&doc)

	if doc.Data.ImplementationName == nil || *doc.Data.ImplementationName != "staging" {
		t.Errorf("implementation_name = %v, want staging", doc.Data.ImplementationName)
	}

	// Refs should be written under the branch.
	refs := readFeatureRefs(t, fx.app.DB, branch.ID, "auth-feature")
	if len(refs) == 0 {
		t.Error("feature_branch_refs should be written for auth-feature")
	}
}
