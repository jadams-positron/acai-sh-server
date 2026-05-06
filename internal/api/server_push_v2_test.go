package api_test

import (
	"context"
	"net/http"
	"testing"
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
