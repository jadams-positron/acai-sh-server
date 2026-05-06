package api

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/api/apierror"
	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/api/spec"
	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
)

// AcaiWebApiImplementationsControllerIndex implements GET /api/v1/implementations.
//
//nolint:revive,staticcheck // method name is dictated by oapi-codegen ServerInterface
func (s *Server) AcaiWebApiImplementationsControllerIndex(c echo.Context, params spec.AcaiWebApiImplementationsControllerIndexParams) error {
	team := middleware.TeamFromEcho(c)
	if team == nil {
		// Auth middleware should have populated this — defensive 401.
		return apierror.WriteAppErrorEcho(c, http.StatusUnauthorized, "missing team context", "")
	}

	listParams := implementations.ListByTeamParams{TeamID: team.ID}

	if params.ProductName != nil && *params.ProductName != "" {
		prod, err := s.products.GetByTeamAndName(c.Request().Context(), team.ID, *params.ProductName)
		if err != nil {
			if products.IsNotFound(err) {
				// Phoenix returns an empty list (200) when product is unknown — match that.
				return c.JSON(http.StatusOK, emptyImplsResponse(params))
			}
			return apierror.WriteAppErrorEcho(c, http.StatusInternalServerError, "failed to look up product", "")
		}
		listParams.ProductID = &prod.ID
	}

	if (params.RepoUri != nil) != (params.BranchName != nil) {
		return apierror.WriteAppErrorEcho(c, http.StatusUnprocessableEntity,
			"repo_uri and branch_name must be provided together", "")
	}
	listParams.RepoURI = params.RepoUri
	listParams.BranchName = params.BranchName

	impls, err := s.implementations.List(c.Request().Context(), listParams)
	if err != nil {
		if implementations.IsInvalidParams(err) {
			return apierror.WriteAppErrorEcho(c, http.StatusUnprocessableEntity, err.Error(), "")
		}
		return apierror.WriteAppErrorEcho(c, http.StatusInternalServerError, "failed to list implementations", "")
	}

	return c.JSON(http.StatusOK, buildImplsResponse(params, impls))
}

func buildImplsResponse(params spec.AcaiWebApiImplementationsControllerIndexParams, impls []*implementations.Implementation) spec.ImplementationsResponse {
	resp := spec.ImplementationsResponse{}
	resp.Data.ProductName = params.ProductName
	resp.Data.RepoUri = params.RepoUri
	resp.Data.BranchName = params.BranchName

	for i := range impls {
		impl := impls[i]
		resp.Data.Implementations = append(resp.Data.Implementations, struct {
			ImplementationId   string `json:"implementation_id"` //nolint:revive,staticcheck // field name dictated by oapi-codegen ImplementationsResponse
			ImplementationName string `json:"implementation_name"`
			ProductName        string `json:"product_name"`
		}{
			ImplementationId:   impl.ID,
			ImplementationName: impl.Name,
			ProductName:        impl.ProductName,
		})
	}

	if resp.Data.Implementations == nil {
		// Ensure we emit JSON `[]` not `null` for empty slices.
		resp.Data.Implementations = []struct {
			ImplementationId   string `json:"implementation_id"` //nolint:revive,staticcheck // field name dictated by oapi-codegen ImplementationsResponse
			ImplementationName string `json:"implementation_name"`
			ProductName        string `json:"product_name"`
		}{}
	}
	return resp
}

func emptyImplsResponse(params spec.AcaiWebApiImplementationsControllerIndexParams) spec.ImplementationsResponse {
	return buildImplsResponse(params, nil)
}
