package auth

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
)

const (
	scopeKey = "acai.scope"
)

// LoadScope reads user_id from the session, fetches the user, and stores a
// *Scope on the echo context. Anonymous requests get an empty Scope.
func LoadScope(store *SessionStore, repo *accounts.Repository) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			scope := &Scope{}
			if userID := store.CurrentUserID(c); userID != "" {
				user, err := repo.GetUserByID(c.Request().Context(), userID)
				if err == nil {
					scope.User = user
				} else if accounts.IsNotFound(err) {
					_ = store.Logout(c)
				}
			}
			c.Set(scopeKey, scope)
			return next(c)
		}
	}
}

// ScopeFromEcho returns the scope from the echo context.
func ScopeFromEcho(c echo.Context) *Scope {
	if s, ok := c.Get(scopeKey).(*Scope); ok && s != nil {
		return s
	}
	return &Scope{}
}

// RequireAuth redirects unauthenticated requests to /users/log-in.
func RequireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !ScopeFromEcho(c).IsAuthenticated() {
			return c.Redirect(http.StatusSeeOther, "/users/log-in")
		}
		return next(c)
	}
}

// RedirectIfAuth redirects authenticated requests to /teams.
func RedirectIfAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if ScopeFromEcho(c).IsAuthenticated() {
			return c.Redirect(http.StatusSeeOther, "/teams")
		}
		return next(c)
	}
}
