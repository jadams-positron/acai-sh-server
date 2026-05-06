// Package api owns the /api/v1 sub-router: bearer auth, size cap, rate limit,
// and operation registration via huma. P2a stands up the scaffold; P2b/P2c add
// individual operations.
package api

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/api/operations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
)

// Deps groups the dependencies the api sub-router needs.
type Deps struct {
	Teams      *teams.Repository
	Operations *operations.Config
	Limiter    middleware.Limiter
}

// applySecurityScheme patches an existing huma.Config's OpenAPI spec with the
// bearer auth security scheme and server URL. Safe to call after DefaultConfig.
func applySecurityScheme(cfg *huma.Config) {
	cfg.Servers = []*huma.Server{{URL: "/api/v1", Description: "API v1"}}
	if cfg.Components == nil {
		cfg.Components = &huma.Components{}
	}
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: "API token"},
	}
	cfg.Security = []map[string][]string{{"bearerAuth": {}}}
}

// publicHumaConfig builds the OpenAPI 3.1 config for the public spec group
// (serves /openapi.json, no docs UI, no auth middleware).
func publicHumaConfig() huma.Config {
	cfg := huma.DefaultConfig("Acai API", "1.0.0")
	applySecurityScheme(&cfg)
	cfg.OpenAPIPath = "/openapi" // huma appends .json → serves /openapi.json
	cfg.DocsPath = ""            // suppress huma's bundled docs UI
	return cfg
}

// authedHumaConfig builds the OpenAPI 3.1 config for the auth'd huma instance.
// Spec serving is disabled here to avoid registering duplicate routes with the
// public group; the public /openapi.json is the canonical spec endpoint.
func authedHumaConfig() huma.Config {
	cfg := huma.DefaultConfig("Acai API", "1.0.0")
	applySecurityScheme(&cfg)
	cfg.OpenAPIPath = "" // no spec routes on the auth'd adapter
	cfg.DocsPath = ""    // suppress huma's bundled docs UI
	return cfg
}

// Mount registers the /api/v1/* routes on parent. P2a wires only the public
// /openapi.json + the auth'd middleware stack; P2b/P2c register operations.
//
// Returns the auth'd huma.API so subsequent phases can register operations.
func Mount(parent chi.Router, deps *Deps) huma.API {
	var authedAPI huma.API

	parent.Route("/api/v1", func(r chi.Router) {
		// Public openapi.json — no auth. Uses publicHumaConfig which enables
		// spec serving; humachi registers /openapi.json on this group's router
		// which chi maps to GET /api/v1/openapi.json from the outer perspective.
		r.Group(func(r chi.Router) {
			pubAPI := humachi.New(r, publicHumaConfig())
			_ = pubAPI // operations registered later in P2b/P2c
		})

		// Authenticated group: bearer + size cap + rate limit, then huma.
		// Uses authedHumaConfig (no spec routes) to avoid duplicate-route
		// conflicts with the public group above.
		r.Group(func(r chi.Router) {
			r.Use(middleware.BearerAuth(deps.Teams))
			r.Use(middleware.SizeCap(deps.Operations.SizeCapForPath))
			r.Use(middleware.RateLimit(deps.Operations.RateLimitForPath, deps.Limiter))

			authedAPI = humachi.New(r, authedHumaConfig())
		})
	})

	return authedAPI
}
