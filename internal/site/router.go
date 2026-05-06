// Package site mounts browser-facing HTTP routes.
package site

import (
	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/site/handlers"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// csrfTokenFromEcho returns the CSRF token echo's middleware injected.
func csrfTokenFromEcho(c echo.Context) string {
	if tok, ok := c.Get("csrf").(string); ok {
		return tok
	}
	return ""
}

// MountAuthRoutes registers the login/logout routes on the group. Caller is
// expected to have mounted auth.LoadScope at the parent group level.
//
// CSRF is applied on the auth subtree (everywhere except the magic-link
// confirm GET, where the token IS the auth proof).
func MountAuthRoutes(g *echo.Group, deps *handlers.AuthDeps, csrfMiddleware echo.MiddlewareFunc) {
	// Routes for unauthenticated users only.
	unauth := g.Group("", csrfMiddleware, auth.RedirectIfAuth)
	unauth.GET("/users/log-in", handlers.LoginNew(deps))
	unauth.POST("/users/log-in", handlers.LoginCreate(deps))
	unauth.GET("/users/register", handlers.RegisterNew(deps))
	unauth.POST("/users/register", handlers.RegisterCreate(deps))

	// Magic-link consume — bypasses CSRF (token IS the proof).
	g.GET("/users/log-in/:token", handlers.LoginConfirm(deps))

	// Logout (CSRF protected).
	logout := g.Group("", csrfMiddleware)
	logout.POST("/users/log-out", handlers.LogOut(deps))
}

// MountAuthRequiredStub mounts /teams as a P1d proof-of-life endpoint.
func MountAuthRequiredStub(g *echo.Group, csrfMiddleware echo.MiddlewareFunc) {
	authd := g.Group("", csrfMiddleware, auth.RequireAuth)
	authd.GET("/teams", func(c echo.Context) error {
		s := auth.ScopeFromEcho(c)
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return views.TeamsStub(views.TeamsStubProps{
			UserEmail: s.User.Email,
			CSRFToken: csrfTokenFromEcho(c),
		}).Render(c.Request().Context(), c.Response())
	})
}
