package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
)

func newCtxWithScope(t *testing.T, scope *auth.Scope) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "/x", http.NoBody)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if scope != nil {
		c.Set("acai.scope", scope)
	}
	return c, rec
}

func TestRequireAuth_RedirectsAnonymous(t *testing.T) {
	c, rec := newCtxWithScope(t, nil)
	called := false
	h := auth.RequireAuth(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})
	if err := h(c); err != nil {
		t.Fatalf("h: %v", err)
	}
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/users/log-in" {
		t.Errorf("Location = %q", loc)
	}
	if called {
		t.Errorf("downstream should not have been called")
	}
}

func TestRequireAuth_AllowsAuthenticated(t *testing.T) {
	c, rec := newCtxWithScope(t, &auth.Scope{User: &accounts.User{ID: "u1"}})
	called := false
	h := auth.RequireAuth(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})
	if err := h(c); err != nil {
		t.Fatalf("h: %v", err)
	}
	if !called || rec.Code != http.StatusOK {
		t.Errorf("called=%v code=%d", called, rec.Code)
	}
}

func TestRedirectIfAuth_RedirectsAuthenticated(t *testing.T) {
	c, rec := newCtxWithScope(t, &auth.Scope{User: &accounts.User{ID: "u1"}})
	called := false
	h := auth.RedirectIfAuth(func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})
	if err := h(c); err != nil {
		t.Fatalf("h: %v", err)
	}
	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", rec.Code)
	}
	if called {
		t.Errorf("downstream should not have been called")
	}
}
