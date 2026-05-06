package api_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

// setupImpl is the shared fixture for all /api/v1/implementations tests.
// Returns an App with: 1 team + 1 user + 1 access-token + 1 product +
// 2 implementations (impl1 tracks repo=github.com/foo/bar branch=main, impl2 does not).
type setupImplResult struct {
	app       *testfx.App
	plaintext string
	impl1ID   string
	impl2ID   string
}

func setupImpl(t *testing.T) *setupImplResult {
	t.Helper()
	app := testfx.NewApp(t, testfx.NewAppOpts{})
	user := testfx.SeedUser(t, app.DB, testfx.SeedUserOpts{})
	team := testfx.SeedTeam(t, app.DB, testfx.SeedTeamOpts{Name: "alpha"})
	_, plaintext := testfx.SeedAccessToken(t, app.DB, user, team, testfx.SeedAccessTokenOpts{})
	product := testfx.SeedProduct(t, app.DB, team, testfx.SeedProductOpts{Name: "myapp"})
	impl1 := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "production"})
	impl2 := testfx.SeedImplementation(t, app.DB, product, testfx.SeedImplementationOpts{Name: "staging"})
	branch := testfx.SeedBranch(t, app.DB, team, testfx.SeedBranchOpts{
		RepoURI:    "github.com/foo/bar",
		BranchName: "main",
	})
	testfx.SeedTrackedBranch(t, app.DB, impl1, branch)
	return &setupImplResult{
		app:       app,
		plaintext: plaintext,
		impl1ID:   impl1.ID,
		impl2ID:   impl2.ID,
	}
}

func TestImplementationsList_NoFilters_ReturnsAll(t *testing.T) {
	fx := setupImpl(t)
	resp := fx.app.Client().WithBearer(fx.plaintext).GET("/api/v1/implementations", nil)
	resp.AssertStatus(http.StatusOK)

	var doc struct {
		Data struct {
			Implementations []struct {
				ImplementationID   string `json:"implementation_id"`
				ImplementationName string `json:"implementation_name"`
				ProductName        string `json:"product_name"`
			} `json:"implementations"`
		} `json:"data"`
	}
	resp.JSON(&doc)
	if len(doc.Data.Implementations) != 2 {
		t.Errorf("got %d implementations, want 2", len(doc.Data.Implementations))
	}
	for _, impl := range doc.Data.Implementations {
		if impl.ProductName != "myapp" {
			t.Errorf("ProductName = %q, want myapp", impl.ProductName)
		}
	}
}

func TestImplementationsList_FilterByProduct_KnownProduct(t *testing.T) {
	fx := setupImpl(t)
	resp := fx.app.Client().WithBearer(fx.plaintext).GET("/api/v1/implementations", url.Values{"product_name": {"myapp"}})
	resp.AssertStatus(http.StatusOK)

	var doc map[string]any
	resp.JSON(&doc)
	data := doc["data"].(map[string]any)
	if data["product_name"] != "myapp" {
		t.Errorf("product_name = %v, want myapp", data["product_name"])
	}
	if len(data["implementations"].([]any)) != 2 {
		t.Errorf("got %d, want 2", len(data["implementations"].([]any)))
	}
}

func TestImplementationsList_FilterByProduct_UnknownProduct(t *testing.T) {
	fx := setupImpl(t)
	resp := fx.app.Client().WithBearer(fx.plaintext).GET("/api/v1/implementations", url.Values{"product_name": {"does-not-exist"}})
	resp.AssertStatus(http.StatusOK)

	var doc map[string]any
	resp.JSON(&doc)
	data := doc["data"].(map[string]any)
	impls, _ := data["implementations"].([]any)
	if len(impls) != 0 {
		t.Errorf("got %d, want 0", len(impls))
	}
}

func TestImplementationsList_FilterByBranch(t *testing.T) {
	fx := setupImpl(t)
	resp := fx.app.Client().WithBearer(fx.plaintext).GET("/api/v1/implementations", url.Values{
		"repo_uri":    {"github.com/foo/bar"},
		"branch_name": {"main"},
	})
	resp.AssertStatus(http.StatusOK)

	var doc map[string]any
	resp.JSON(&doc)
	data := doc["data"].(map[string]any)
	impls := data["implementations"].([]any)
	if len(impls) != 1 {
		t.Fatalf("got %d, want 1 (only impl1 tracks the branch)", len(impls))
	}
	first := impls[0].(map[string]any)
	if first["implementation_name"] != "production" {
		t.Errorf("implementation_name = %v, want production", first["implementation_name"])
	}
}

func TestImplementationsList_RepoWithoutBranch_422(t *testing.T) {
	fx := setupImpl(t)
	resp := fx.app.Client().WithBearer(fx.plaintext).GET("/api/v1/implementations", url.Values{
		"repo_uri": {"github.com/foo/bar"},
	})
	resp.AssertStatus(http.StatusUnprocessableEntity)

	if !strings.Contains(string(resp.Body()), "branch_name") {
		t.Errorf("body should mention branch_name; got %s", resp.Body())
	}
}

func TestImplementationsList_NoBearer_401(t *testing.T) {
	fx := setupImpl(t)
	resp := fx.app.Client().GET("/api/v1/implementations", nil)
	resp.AssertStatus(http.StatusUnauthorized)
}
