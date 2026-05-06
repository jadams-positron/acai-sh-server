// Package site mounts browser-facing HTTP routes.
package site

import (
	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/site/handlers"
)

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

// MountTeamsRoutes registers /teams GET and POST on the group.
func MountTeamsRoutes(g *echo.Group, deps *handlers.TeamsDeps, csrfMiddleware echo.MiddlewareFunc) {
	authd := g.Group("", csrfMiddleware, auth.RequireAuth)
	authd.GET("/teams", handlers.TeamsIndex(deps))
	authd.POST("/teams", handlers.TeamsCreate(deps))
}

// MountTeamShowRoutes registers the team detail routes (/t/:team_name, /t/:team_name/products).
func MountTeamShowRoutes(g *echo.Group, deps *handlers.TeamShowDeps, csrfMiddleware echo.MiddlewareFunc) {
	authd := g.Group("", csrfMiddleware, auth.RequireAuth)
	authd.GET("/t/:team_name", handlers.TeamShow(deps))
	authd.POST("/t/:team_name/products", handlers.TeamCreateProduct(deps))
}

// MountProductShowRoutes registers the product detail route (/t/:team_name/p/:product_name).
func MountProductShowRoutes(g *echo.Group, deps *handlers.ProductShowDeps, csrfMiddleware echo.MiddlewareFunc) {
	authd := g.Group("", csrfMiddleware, auth.RequireAuth)
	authd.GET("/t/:team_name/p/:product_name", handlers.ProductShow(deps))
}

// MountFeatureShowRoutes registers the feature dashboard route (/t/:team_name/f/:feature_name).
func MountFeatureShowRoutes(g *echo.Group, deps *handlers.FeatureShowDeps, csrfMiddleware echo.MiddlewareFunc) {
	authd := g.Group("", csrfMiddleware, auth.RequireAuth)
	authd.GET("/t/:team_name/f/:feature_name", handlers.FeatureShow(deps))
}
