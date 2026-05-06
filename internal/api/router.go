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
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
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
	Teams      *teams.Repository
	Operations *operations.Config
	Limiter    middleware.Limiter
}

// Mount registers /api/v1/* routes on the parent echo. Public:
// /api/v1/openapi.json. Authed: bearer + size-cap + rate-limit, then the
// generated ServerInterface routes (501 stubs in P2a).
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

	// Register the generated ServerInterface using a stub server.
	// In P2b/P2c, the real Server implementation replaces unimplementedServer.
	spec.RegisterHandlers(authd, &unimplementedServer{})
}

// unimplementedServer satisfies spec.ServerInterface with 501 stubs.
type unimplementedServer struct{}

func (unimplementedServer) AcaiWebApiFeatureContextControllerShow(c echo.Context, _ spec.AcaiWebApiFeatureContextControllerShowParams) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "feature-context not implemented yet (P2b)")
}

func (unimplementedServer) AcaiWebApiFeatureStatesControllerUpdate(c echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "feature-states not implemented yet (P2c)")
}

func (unimplementedServer) AcaiWebApiImplementationFeaturesControllerIndex(c echo.Context, _ spec.AcaiWebApiImplementationFeaturesControllerIndexParams) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "implementation-features not implemented yet (P2b)")
}

func (unimplementedServer) AcaiWebApiImplementationsControllerIndex(c echo.Context, _ spec.AcaiWebApiImplementationsControllerIndexParams) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "implementations not implemented yet (P2b)")
}

func (unimplementedServer) AcaiWebApiPushControllerCreate(c echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "push not implemented yet (P2c)")
}
