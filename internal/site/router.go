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

	// Google SSO. Login starts the redirect (CSRF protected — same-origin
	// form submit from the login page). Callback is the OIDC redirect from
	// Google itself, so it bypasses our CSRF middleware; defends in depth
	// via the state cookie + the OIDC nonce.
	if deps.Google != nil {
		unauthGoogle := g.Group("", csrfMiddleware, auth.RedirectIfAuth)
		unauthGoogle.POST("/auth/google/login", handlers.GoogleLogin(deps))
		g.GET("/auth/google/callback", handlers.GoogleCallback(deps))
	}

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

// MountImplFeatureShowRoutes registers the impl×feature drill-down route.
func MountImplFeatureShowRoutes(g *echo.Group, deps *handlers.ImplFeatureShowDeps, csrfMiddleware echo.MiddlewareFunc) {
	authd := g.Group("", csrfMiddleware, auth.RequireAuth)
	authd.GET("/t/:team_name/i/:impl_slug/f/:feature_name", handlers.ImplFeatureShow(deps))
	authd.POST("/t/:team_name/i/:impl_slug/f/:feature_name/acid/:acid/status", handlers.ImplFeatureSetStatus(deps))
}

// MountImplsIndexRoutes registers the /t/:team_name/implementations route.
func MountImplsIndexRoutes(g *echo.Group, deps *handlers.ImplsIndexDeps, csrfMiddleware echo.MiddlewareFunc) {
	authd := g.Group("", csrfMiddleware, auth.RequireAuth)
	authd.GET("/t/:team_name/implementations", handlers.ImplsIndex(deps))
}

// MountFeaturesIndexRoutes registers the /t/:team_name/features cross-product index route.
func MountFeaturesIndexRoutes(g *echo.Group, deps *handlers.FeaturesIndexDeps, csrfMiddleware echo.MiddlewareFunc) {
	authd := g.Group("", csrfMiddleware, auth.RequireAuth)
	authd.GET("/t/:team_name/features", handlers.FeaturesIndex(deps))
}

// MountSearchRoutes registers the /t/:team_name/search JSON endpoint.
func MountSearchRoutes(g *echo.Group, deps *handlers.SearchDeps) {
	authd := g.Group("", auth.RequireAuth)
	authd.GET("/t/:team_name/search", handlers.Search(deps))
}

// MountBranchesIndexRoutes registers the /t/:team_name/branches index route.
func MountBranchesIndexRoutes(g *echo.Group, deps *handlers.BranchesIndexDeps, csrfMiddleware echo.MiddlewareFunc) {
	authd := g.Group("", csrfMiddleware, auth.RequireAuth)
	authd.GET("/t/:team_name/branches", handlers.BranchesIndex(deps))
}

// MountImplShowRoutes registers the /t/:team_name/i/:impl_slug detail route.
func MountImplShowRoutes(g *echo.Group, deps *handlers.ImplShowDeps, csrfMiddleware echo.MiddlewareFunc) {
	authd := g.Group("", csrfMiddleware, auth.RequireAuth)
	authd.GET("/t/:team_name/i/:impl_slug", handlers.ImplShow(deps))
}

// MountTeamSettingsRoutes registers the team settings routes.
func MountTeamSettingsRoutes(g *echo.Group, deps *handlers.TeamSettingsDeps, csrfMiddleware echo.MiddlewareFunc) {
	authd := g.Group("", csrfMiddleware, auth.RequireAuth)
	authd.GET("/t/:team_name/settings", handlers.TeamSettings(deps))
	authd.POST("/t/:team_name/settings/members", handlers.TeamSettingsAddMember(deps))
	authd.POST("/t/:team_name/settings/members/:user_id/remove", handlers.TeamSettingsRemoveMember(deps))
}

// MountTeamTokensRoutes registers the team token management routes.
func MountTeamTokensRoutes(g *echo.Group, deps *handlers.TeamTokensDeps, csrfMiddleware echo.MiddlewareFunc) {
	authd := g.Group("", csrfMiddleware, auth.RequireAuth)
	authd.GET("/t/:team_name/tokens", handlers.TeamTokens(deps))
	authd.POST("/t/:team_name/tokens", handlers.TeamTokensCreate(deps))
	authd.POST("/t/:team_name/tokens/:prefix/revoke", handlers.TeamTokensRevoke(deps))
}
