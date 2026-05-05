package server

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/ops"
	"github.com/jadams-positron/acai-sh-server/internal/site"
)

// newRouter builds the chi router with auth + site routes mounted.
func newRouter(deps *RouterDeps) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	// Browser routes get sessions + scope.
	r.Group(func(r chi.Router) {
		r.Use(deps.Sessions.LoadAndSave)
		r.Use(auth.LoadScope(deps.Sessions, deps.Accounts))

		site.MountAuthRoutes(r, deps.AuthHandlerDeps, deps.CSRFKey, deps.SecureCookie)
		site.MountAuthRequiredStub(r)
	})

	// Health check is outside the session middleware.
	r.Method("GET", "/_health", ops.HealthHandler(deps.DB, deps.Version))

	return r
}
