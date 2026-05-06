// Package api owns the /api/v1 sub-router: bearer auth, size cap, rate limit,
// and operation registration via oapi-codegen generated ServerInterface.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/api/operations"
	"github.com/jadams-positron/acai-sh-server/internal/api/spec"
	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	domainspecs "github.com/jadams-positron/acai-sh-server/internal/domain/specs"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/services"
)

// openapiJSON is the canonical OpenAPI spec. It is decoded from the embedded
// gzipped+base64 data in spec.gen.go and re-serialized at init time.
var openapiJSON = func() []byte {
	swagger, err := spec.GetSwagger()
	if err != nil {
		panic("api: spec.GetSwagger: " + err.Error())
	}
	b, err := json.Marshal(swagger)
	if err != nil {
		panic("api: marshal swagger: " + err.Error())
	}
	return b
}()

// Deps groups the dependencies the api sub-router needs.
type Deps struct {
	Teams           *teams.Repository
	Products        *products.Repository
	Implementations *implementations.Repository
	Specs           *domainspecs.Repository
	Operations      *operations.Config
	Limiter         middleware.Limiter
}

// Mount registers /api/v1/* routes on the parent echo. Public:
// /api/v1/openapi.json. Authed: bearer + size-cap + rate-limit, then the
// generated ServerInterface routes.
func Mount(parent *echo.Echo, deps *Deps) {
	v1 := parent.Group("/api/v1")

	// Public openapi.json — serve the embedded source-of-truth spec verbatim.
	v1.GET("/openapi.json", func(c echo.Context) error {
		return c.Blob(http.StatusOK, "application/json", openapiJSON)
	})

	// Authenticated group.
	authd := v1.Group("",
		middleware.BearerAuth(deps.Teams),
		middleware.SizeCap(deps.Operations.SizeCapForPath),
		middleware.RateLimit(deps.Operations.RateLimitForPath, deps.Limiter),
	)

	fcSvc := services.NewFeatureContextService(deps.Products, deps.Implementations, deps.Specs)
	ifSvc := services.NewImplementationFeaturesService(deps.Products, deps.Implementations, deps.Specs)
	fsSvc := services.NewFeatureStatesService(deps.Products, deps.Implementations, deps.Specs)
	pushSvc := services.NewPushService(deps.Products, deps.Implementations, deps.Specs)
	srv := &Server{
		products:        deps.Products,
		implementations: deps.Implementations,
		featureContext:  fcSvc,
		implFeatures:    ifSvc,
		featureStates:   fsSvc,
		push:            pushSvc,
		operations:      deps.Operations,
	}
	spec.RegisterHandlers(authd, srv)
}

// Server is the concrete spec.ServerInterface implementation. All 5 /api/v1/*
// methods are implemented in their own file (e.g. server_implementations.go,
// server_push.go).
type Server struct {
	products        *products.Repository
	implementations *implementations.Repository
	featureContext  *services.FeatureContextService
	implFeatures    *services.ImplementationFeaturesService
	featureStates   *services.FeatureStatesService
	push            *services.PushService
	operations      *operations.Config
}
