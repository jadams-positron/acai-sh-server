package server

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jadams-positron/acai-sh-server/internal/api"
	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/ops"
	"github.com/jadams-positron/acai-sh-server/internal/site"
	"github.com/jadams-positron/acai-sh-server/internal/site/handlers"
)

// newRouter builds the chi router with auth + site routes mounted.
func newRouter(deps *RouterDeps) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	// Static assets are public — mount before session-loaded group.
	handlers.MountStatic(r)

	// Browser routes get sessions + scope.
	r.Group(func(r chi.Router) {
		r.Use(deps.Sessions.LoadAndSave)
		r.Use(auth.LoadScope(deps.Sessions, deps.Accounts))

		site.MountAuthRoutes(r, deps.AuthHandlerDeps, deps.CSRFKey, deps.SecureCookie)
		site.MountAuthRequiredStub(r, deps.CSRFKey, deps.SecureCookie)
	})

	// Health check is outside the session middleware.
	r.Method("GET", "/_health", ops.HealthHandler(deps.DB, deps.Version))

	// API sub-router: bearer auth, size cap, rate limit, huma.
	api.Mount(r, &api.Deps{
		Teams:      deps.Teams,
		Operations: deps.Operations,
		Limiter:    deps.APILimiter,
	})

	return r
}
