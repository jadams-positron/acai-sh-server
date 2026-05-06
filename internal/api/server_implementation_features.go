package api

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/api/apierror"
	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/api/spec"
	"github.com/jadams-positron/acai-sh-server/internal/services"
)

// AcaiWebApiImplementationFeaturesControllerIndex implements GET /api/v1/implementation-features.
//
//nolint:revive,staticcheck // method name is dictated by oapi-codegen ServerInterface
func (s *Server) AcaiWebApiImplementationFeaturesControllerIndex(c echo.Context, params spec.AcaiWebApiImplementationFeaturesControllerIndexParams) error {
	team := middleware.TeamFromEcho(c)
	if team == nil {
		return apierror.WriteAppErrorEcho(c, http.StatusUnauthorized, "missing team context", "")
	}

	req := services.ListFeaturesRequest{
		Team:               team,
		ProductName:        params.ProductName,
		ImplementationName: params.ImplementationName,
	}
	if params.Statuses != nil {
		req.StatusFilter = *params.Statuses
	}
	if params.ChangedSinceCommit != nil {
		req.ChangedSinceCommit = params.ChangedSinceCommit
	}

	result, err := s.implFeatures.List(c.Request().Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrProductNotFound):
			return apierror.WriteAppErrorEcho(c, http.StatusNotFound, "product not found", "")
		case errors.Is(err, services.ErrImplementationNotFound):
			return apierror.WriteAppErrorEcho(c, http.StatusNotFound, "implementation not found", "")
		default:
			return apierror.WriteAppErrorEcho(c, http.StatusInternalServerError, "failed to list implementation features", "")
		}
	}

	return c.JSON(http.StatusOK, buildImplementationFeaturesResponse(result))
}

// implementationFeaturesData mirrors spec.ImplementationFeaturesResponse.Data as
// a named type to avoid Go's anonymous-struct assignability rules.
type implementationFeaturesData struct {
	Features           []implFeatureEntry `json:"features"`
	ImplementationId   string             `json:"implementation_id"`   //nolint:revive,staticcheck // ST1003/var-naming: matches generated JSON tag
	ImplementationName string             `json:"implementation_name"` //nolint:revive,staticcheck // ST1003/var-naming: matches generated JSON tag
	ProductName        string             `json:"product_name"`
}

type implFeatureEntry struct {
	CompletedCount     int     `json:"completed_count"`
	Description        *string `json:"description"`
	FeatureName        string  `json:"feature_name"`
	HasLocalSpec       bool    `json:"has_local_spec"`
	HasLocalStates     bool    `json:"has_local_states"`
	RefsCount          int     `json:"refs_count"`
	RefsInherited      bool    `json:"refs_inherited"`
	SpecLastSeenCommit *string `json:"spec_last_seen_commit"`
	StatesInherited    bool    `json:"states_inherited"`
	TestRefsCount      int     `json:"test_refs_count"`
	TotalCount         int     `json:"total_count"`
}

// implementationFeaturesResponse wraps implementationFeaturesData so json.Marshal
// emits {"data":{...}}.
type implementationFeaturesResponse struct {
	Data implementationFeaturesData `json:"data"`
}

// buildImplementationFeaturesResponse composes the response from the resolved ListResult.
func buildImplementationFeaturesResponse(result *services.ListResult) implementationFeaturesResponse {
	entries := make([]implFeatureEntry, 0, len(result.Features))
	for _, f := range result.Features {
		entries = append(entries, implFeatureEntry{
			CompletedCount:     f.CompletedCount,
			Description:        f.Description,
			FeatureName:        f.FeatureName,
			HasLocalSpec:       f.HasLocalSpec,
			HasLocalStates:     f.HasLocalStates,
			RefsCount:          f.RefsCount,
			RefsInherited:      f.RefsInherited,
			SpecLastSeenCommit: f.SpecLastSeenCommit,
			StatesInherited:    f.StatesInherited,
			TestRefsCount:      f.TestRefsCount,
			TotalCount:         f.TotalCount,
		})
	}

	return implementationFeaturesResponse{
		Data: implementationFeaturesData{
			Features:           entries,
			ImplementationId:   result.Implementation.ID,
			ImplementationName: result.Implementation.Name,
			ProductName:        result.Product.Name,
		},
	}
}
