// Package server wires together the HTTP server, chi router, and middleware.
package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/acai-sh/server/internal/ops"
	"github.com/acai-sh/server/internal/store"
)

// newRouter builds the chi router with standard middleware and all route mounts.
func newRouter(db *store.DB, version string) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Method(http.MethodGet, "/_health", ops.HealthHandler(db, version))

	return r
}
