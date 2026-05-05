package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/acai-sh/server/internal/auth"
	"github.com/acai-sh/server/internal/domain/accounts"
)

func TestRequireAuth_RedirectsAnonymous(t *testing.T) {
	handler := auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("downstream handler should not have been called")
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/teams", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/users/log-in" {
		t.Errorf("Location = %q, want /users/log-in", loc)
	}
}

func TestRequireAuth_AllowsAuthenticated(t *testing.T) {
	called := false
	handler := auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/teams", http.NoBody)
	ctx := auth.WithScope(req.Context(), &auth.Scope{User: &accounts.User{ID: "u1"}})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Errorf("downstream handler not called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRedirectIfAuth_RedirectsAuthenticated(t *testing.T) {
	handler := auth.RedirectIfAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("downstream handler should not have been called")
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/users/log-in", http.NoBody)
	ctx := auth.WithScope(req.Context(), &auth.Scope{User: &accounts.User{ID: "u1"}})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/teams" {
		t.Errorf("Location = %q, want /teams", loc)
	}
}
