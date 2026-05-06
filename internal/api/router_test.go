package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/api"
	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/api/operations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

func setupAPI(t *testing.T) *echo.Echo {
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

	e := echo.New()
	api.Mount(e, &api.Deps{
		Teams:           teams.NewRepository(db),
		Products:        products.NewRepository(db),
		Implementations: implementations.NewRepository(db),
		Operations:      operations.Load(true),
		Limiter:         middleware.NewInProcessLimiter(),
	})
	return e
}

func TestMount_OpenAPIJSONPublicAndValid(t *testing.T) {
	e := setupAPI(t)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/openapi.json", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc["openapi"] == nil {
		t.Errorf("openapi key missing in response")
	}
	if doc["info"] == nil {
		t.Errorf("info key missing in response")
	}
}

func TestMount_AuthedEndpoint_NoAuth_Returns401(t *testing.T) {
	e := setupAPI(t)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/implementations", http.NoBody)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rec.Code, rec.Body.String())
	}
}
