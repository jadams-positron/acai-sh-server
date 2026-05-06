package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/api"
	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/api/operations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

// implFixtures sets up: 1 team + 1 user + 1 access-token + 1 product +
// 2 implementations (one tracking a branch, one not).
type implFixtures struct {
	plaintext string
	teamID    string
	productID string
	impl1ID   string // tracks repo=github.com/foo/bar, branch=main
	impl2ID   string // no tracked branch
}

func setupImplFixtures(t *testing.T) (*echo.Echo, *implFixtures) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	ar := accounts.NewRepository(db)
	tr := teams.NewRepository(db)
	pr := products.NewRepository(db)
	ir := implementations.NewRepository(db)

	user, err := ar.CreateUser(context.Background(), accounts.CreateUserParams{Email: "u@example.com"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	team, err := tr.CreateTeam(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	plaintext, err := tr.CreateAccessToken(context.Background(), teams.CreateAccessTokenParams{
		UserID: user.ID,
		TeamID: team.ID,
		Name:   "test-token",
	})
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}

	// Insert product, two implementations, a branch, and a tracked_branches entry.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	productID := uuid.New().String()
	if _, err := db.Write.ExecContext(context.Background(),
		"INSERT INTO products (id, team_id, name, is_active, inserted_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)",
		productID, team.ID, "myapp", now, now); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	impl1ID := uuid.New().String()
	impl2ID := uuid.New().String()
	for _, p := range []struct{ id, name string }{{impl1ID, "production"}, {impl2ID, "staging"}} {
		if _, err := db.Write.ExecContext(context.Background(),
			"INSERT INTO implementations (id, product_id, team_id, name, is_active, inserted_at, updated_at) VALUES (?, ?, ?, ?, 1, ?, ?)",
			p.id, productID, team.ID, p.name, now, now); err != nil {
			t.Fatalf("insert implementation: %v", err)
		}
	}
	branchID := uuid.New().String()
	if _, err := db.Write.ExecContext(context.Background(),
		"INSERT INTO branches (id, team_id, repo_uri, branch_name, last_seen_commit, inserted_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		branchID, team.ID, "github.com/foo/bar", "main", "abc", now, now); err != nil {
		t.Fatalf("insert branch: %v", err)
	}
	if _, err := db.Write.ExecContext(context.Background(),
		"INSERT INTO tracked_branches (implementation_id, branch_id, repo_uri, inserted_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		impl1ID, branchID, "github.com/foo/bar", now, now); err != nil {
		t.Fatalf("insert tracked_branches: %v", err)
	}

	e := echo.New()
	api.Mount(e, &api.Deps{
		Teams:           tr,
		Products:        pr,
		Implementations: ir,
		Operations:      operations.Load(true),
		Limiter:         middleware.NewInProcessLimiter(),
	})

	return e, &implFixtures{
		plaintext: plaintext,
		teamID:    team.ID,
		productID: productID,
		impl1ID:   impl1ID,
		impl2ID:   impl2ID,
	}
}

func setBearer(req *http.Request, plaintext string) {
	req.Header.Set("Authorization", "Bearer "+plaintext)
}

func TestImplementationsList_NoFilters_ReturnsAll(t *testing.T) {
	e, fx := setupImplFixtures(t)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/implementations", http.NoBody)
	setBearer(req, fx.plaintext)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var doc struct {
		Data struct {
			Implementations []struct {
				ImplementationID   string `json:"implementation_id"`
				ImplementationName string `json:"implementation_name"`
				ProductName        string `json:"product_name"`
			} `json:"implementations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(doc.Data.Implementations) != 2 {
		t.Errorf("got %d implementations, want 2", len(doc.Data.Implementations))
	}
	for _, impl := range doc.Data.Implementations {
		if impl.ProductName != "myapp" {
			t.Errorf("ProductName = %q, want %q", impl.ProductName, "myapp")
		}
	}
}

func TestImplementationsList_FilterByProduct_KnownProduct(t *testing.T) {
	e, fx := setupImplFixtures(t)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/implementations?product_name=myapp", http.NoBody)
	setBearer(req, fx.plaintext)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var doc map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &doc)
	data := doc["data"].(map[string]any)
	if data["product_name"] != "myapp" {
		t.Errorf("product_name = %v, want myapp", data["product_name"])
	}
	if len(data["implementations"].([]any)) != 2 {
		t.Errorf("got %d, want 2", len(data["implementations"].([]any)))
	}
}

func TestImplementationsList_FilterByProduct_UnknownProduct(t *testing.T) {
	e, fx := setupImplFixtures(t)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/implementations?product_name=does-not-exist", http.NoBody)
	setBearer(req, fx.plaintext)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (unknown product → empty list)", rec.Code)
	}
	var doc map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &doc)
	data := doc["data"].(map[string]any)
	impls, _ := data["implementations"].([]any)
	if len(impls) != 0 {
		t.Errorf("got %d, want 0", len(impls))
	}
}

func TestImplementationsList_FilterByBranch(t *testing.T) {
	e, fx := setupImplFixtures(t)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/implementations?repo_uri=github.com/foo/bar&branch_name=main", http.NoBody)
	setBearer(req, fx.plaintext)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var doc map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &doc)
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
	e, fx := setupImplFixtures(t)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/implementations?repo_uri=github.com/foo/bar", http.NoBody)
	setBearer(req, fx.plaintext)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "branch_name") {
		t.Errorf("body should mention branch_name; got %s", rec.Body.String())
	}
}

func TestImplementationsList_NoBearer_401(t *testing.T) {
	e, _ := setupImplFixtures(t)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/implementations", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
