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
	srv := &Server{
		products:        deps.Products,
		implementations: deps.Implementations,
		featureContext:  fcSvc,
	}
	spec.RegisterHandlers(authd, srv)
}

// Server is the concrete spec.ServerInterface implementation. Methods that are
// implemented live in their own file (e.g. server_implementations.go); the
// rest are inherited from the embedded unimplementedServer.
type Server struct {
	unimplementedServer
	products        *products.Repository
	implementations *implementations.Repository
	featureContext  *services.FeatureContextService
}

// unimplementedServer satisfies spec.ServerInterface with 501 stubs.
// Method names follow oapi-codegen conventions (AcaiWebApi…) to match the
// generated interface; revive var-naming warnings are suppressed because the
// names are dictated by the code generator.
type unimplementedServer struct{}

//nolint:revive,staticcheck // method name is dictated by oapi-codegen ServerInterface
func (unimplementedServer) AcaiWebApiFeatureContextControllerShow(_ echo.Context, _ spec.AcaiWebApiFeatureContextControllerShowParams) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "feature-context not implemented yet (P2b)")
}

//nolint:revive,staticcheck // method name is dictated by oapi-codegen ServerInterface
func (unimplementedServer) AcaiWebApiFeatureStatesControllerUpdate(_ echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "feature-states not implemented yet (P2c)")
}

//nolint:revive,staticcheck // method name is dictated by oapi-codegen ServerInterface
func (unimplementedServer) AcaiWebApiImplementationFeaturesControllerIndex(_ echo.Context, _ spec.AcaiWebApiImplementationFeaturesControllerIndexParams) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "implementation-features not implemented yet (P2b)")
}

//nolint:revive,staticcheck // method name is dictated by oapi-codegen ServerInterface
func (unimplementedServer) AcaiWebApiImplementationsControllerIndex(_ echo.Context, _ spec.AcaiWebApiImplementationsControllerIndexParams) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "implementations not implemented yet (P2b)")
}

//nolint:revive,staticcheck // method name is dictated by oapi-codegen ServerInterface
func (unimplementedServer) AcaiWebApiPushControllerCreate(_ echo.Context) error {
	return echo.NewHTTPError(http.StatusNotImplemented, "push not implemented yet (P2c)")
}
