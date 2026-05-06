package api

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/api/apierror"
	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/api/operations"
	"github.com/jadams-positron/acai-sh-server/internal/api/spec"
	"github.com/jadams-positron/acai-sh-server/internal/services"
)

//nolint:revive,staticcheck // method name is dictated by oapi-codegen ServerInterface
func (s *Server) AcaiWebApiFeatureStatesControllerUpdate(c echo.Context) error {
	team := middleware.TeamFromEcho(c)
	if team == nil {
		return apierror.WriteAppErrorEcho(c, http.StatusUnauthorized, "missing team context", "")
	}

	var body spec.FeatureStatesRequest
	if err := c.Bind(&body); err != nil {
		return apierror.WriteAppErrorEcho(c, http.StatusBadRequest, "invalid JSON body", "")
	}

	// Translate spec.FeatureStatesRequest into services.FeatureStatesUpdate.
	states := make(map[string]services.StateInput, len(body.States))
	for acid, st := range body.States {
		si := services.StateInput{Comment: st.Comment}
		if st.Status != nil {
			ss := string(*st.Status)
			si.Status = &ss
		}
		states[acid] = si
	}

	// Pull semantic caps from the operations config.
	epCfg := s.operations.Endpoints[operations.EndpointFeatureStates]
	update := services.FeatureStatesUpdate{
		Team:               team,
		ProductName:        body.ProductName,
		ImplementationName: body.ImplementationName,
		FeatureName:        body.FeatureName,
		States:             states,
		MaxStates:          epCfg.SemanticCaps["max_states"],
		MaxCommentLength:   epCfg.SemanticCaps["max_comment_length"],
	}

	result, err := s.featureStates.Update(c.Request().Context(), update)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrProductNotFound):
			return apierror.WriteAppErrorEcho(c, http.StatusNotFound, "product not found", "")
		case errors.Is(err, services.ErrImplementationNotFound):
			return apierror.WriteAppErrorEcho(c, http.StatusNotFound, "implementation not found", "")
		case errors.Is(err, services.ErrTooManyStates),
			errors.Is(err, services.ErrCommentTooLong),
			errors.Is(err, services.ErrInvalidStatus):
			return apierror.WriteAppErrorEcho(c, http.StatusUnprocessableEntity, err.Error(), "")
		default:
			return apierror.WriteAppErrorEcho(c, http.StatusInternalServerError, "failed to update feature states", "")
		}
	}

	resp := spec.FeatureStatesResponse{}
	resp.Data.FeatureName = body.FeatureName
	resp.Data.ImplementationId = result.Implementation.ID
	resp.Data.ImplementationName = result.Implementation.Name
	resp.Data.ProductName = result.Product.Name
	resp.Data.StatesWritten = result.StatesWritten
	resp.Data.Warnings = result.Warnings
	if resp.Data.Warnings == nil {
		resp.Data.Warnings = []string{}
	}
	return c.JSON(http.StatusOK, resp)
}
