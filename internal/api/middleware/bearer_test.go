package middleware_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

func setup(t *testing.T) (repo *teams.Repository, plaintext string) {
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
	user, err := ar.CreateUser(context.Background(), accounts.CreateUserParams{Email: "u@example.com"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	team, err := tr.CreateTeam(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	plaintext, err = tr.CreateAccessToken(context.Background(), teams.CreateAccessTokenParams{
		UserID: user.ID, TeamID: team.ID, Name: "test-token",
	})
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}
	return tr, plaintext
}

func runBearer(t *testing.T, repo *teams.Repository, header string) (rec *httptest.ResponseRecorder, called bool) {
	t.Helper()
	e := echo.New()
	e.Use(middleware.BearerAuth(repo))
	e.GET("/api/v1/x", func(c echo.Context) error {
		called = true
		if middleware.TokenFromEcho(c) == nil {
			t.Errorf("downstream: TokenFromEcho returned nil")
		}
		if middleware.TeamFromEcho(c) == nil {
			t.Errorf("downstream: TeamFromEcho returned nil")
		}
		return c.NoContent(http.StatusOK)
	})

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/x", http.NoBody)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec, called
}

func mustErrorEnvelope(t *testing.T, body io.Reader) (detail, status string) {
	t.Helper()
	var doc struct {
		Errors struct {
			Detail string `json:"detail"`
			Status string `json:"status"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(body).Decode(&doc); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return doc.Errors.Detail, doc.Errors.Status
}

func TestBearerAuth_HappyPath(t *testing.T) {
	repo, plaintext := setup(t)
	rec, called := runBearer(t, repo, "Bearer "+plaintext)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Errorf("downstream not called")
	}
}

func TestBearerAuth_MissingHeader(t *testing.T) {
	repo, _ := setup(t)
	rec, called := runBearer(t, repo, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Errorf("downstream should not have been called")
	}
	detail, status := mustErrorEnvelope(t, rec.Body)
	if !strings.Contains(detail, "required") {
		t.Errorf("detail = %q", detail)
	}
	if status != "UNAUTHORIZED" {
		t.Errorf("status = %q", status)
	}
}

func TestBearerAuth_WrongScheme(t *testing.T) {
	repo, _ := setup(t)
	rec, _ := runBearer(t, repo, "Basic dXNlcjpwYXNz")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	detail, _ := mustErrorEnvelope(t, rec.Body)
	if !strings.Contains(detail, "Bearer") {
		t.Errorf("detail = %q", detail)
	}
}

func TestBearerAuth_EmptyToken(t *testing.T) {
	repo, _ := setup(t)
	rec, _ := runBearer(t, repo, "Bearer ")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestBearerAuth_UnknownToken(t *testing.T) {
	repo, _ := setup(t)
	rec, _ := runBearer(t, repo, "Bearer aaaaaaaa.notarealtoken")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	detail, _ := mustErrorEnvelope(t, rec.Body)
	if !strings.Contains(detail, "Invalid") && !strings.Contains(detail, "expired") {
		t.Errorf("detail = %q (expected something about invalid/expired)", detail)
	}
}
