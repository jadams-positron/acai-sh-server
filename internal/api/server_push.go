package api

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/api/apierror"
	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/api/operations"
	"github.com/jadams-positron/acai-sh-server/internal/api/spec"
	"github.com/jadams-positron/acai-sh-server/internal/domain/specs"
	"github.com/jadams-positron/acai-sh-server/internal/services"
)

// AcaiWebApiPushControllerCreate implements POST /api/v1/push.
//
//nolint:revive,staticcheck // method name is dictated by oapi-codegen ServerInterface
func (s *Server) AcaiWebApiPushControllerCreate(c echo.Context) error {
	team := middleware.TeamFromEcho(c)
	if team == nil {
		return apierror.WriteAppErrorEcho(c, http.StatusUnauthorized, "missing team context", "")
	}

	var body spec.AcaiWebApiPushControllerCreateJSONRequestBody
	if err := c.Bind(&body); err != nil {
		return apierror.WriteAppErrorEcho(c, http.StatusBadRequest, "invalid request body", "")
	}

	// Resolve caps from operations config.
	ep := s.operations.Endpoints[operations.EndpointPush]
	caps := ep.SemanticCaps

	req := services.PushRequest{
		Team:           team,
		RepoURI:        body.RepoUri,
		BranchName:     body.BranchName,
		CommitHash:     body.CommitHash,
		ProductName:    body.ProductName,
		TargetImplName: body.TargetImplName,
		ParentImplName: body.ParentImplName,

		MaxSpecs:                    caps["max_specs"],
		MaxReferences:               caps["max_references"],
		MaxRequirementsPerSpec:      caps["max_requirements_per_spec"],
		MaxRawContentBytes:          caps["max_raw_content_bytes"],
		MaxRequirementStringLength:  caps["max_requirement_string_length"],
		MaxFeatureDescriptionLength: caps["max_feature_description_length"],
		MaxMetaPathLength:           caps["max_meta_path_length"],
		MaxRepoURILength:            caps["max_repo_uri_length"],
	}

	// Translate specs.
	if body.Specs != nil {
		for _, sp := range *body.Specs {
			featureVersion := ""
			if sp.Feature.Version != nil {
				featureVersion = *sp.Feature.Version
			}
			reqs := make(map[string]services.RequirementInput, len(sp.Requirements))
			for acid, rd := range sp.Requirements {
				dep := false
				if rd.Deprecated != nil {
					dep = *rd.Deprecated
				}
				var replacedBy []string
				if rd.ReplacedBy != nil {
					replacedBy = *rd.ReplacedBy
				}
				reqs[acid] = services.RequirementInput{
					Requirement: rd.Requirement,
					Deprecated:  dep,
					Note:        rd.Note,
					ReplacedBy:  replacedBy,
				}
			}
			req.Specs = append(req.Specs, services.SpecInput{
				FeatureName:        sp.Feature.Name,
				FeatureProduct:     sp.Feature.Product,
				FeatureDescription: sp.Feature.Description,
				FeatureVersion:     featureVersion,
				Path:               sp.Meta.Path,
				LastSeenCommit:     sp.Meta.LastSeenCommit,
				RawContent:         sp.Meta.RawContent,
				Requirements:       reqs,
			})
		}
	}

	// Translate references.
	if body.References != nil {
		override := false
		if body.References.Override != nil {
			override = *body.References.Override
		}
		refsData := make(map[string][]specs.CodeRef, len(body.References.Data))
		for acid, refs := range body.References.Data {
			codeRefs := make([]specs.CodeRef, 0, len(refs))
			for _, r := range refs {
				isTest := false
				if r.IsTest != nil {
					isTest = *r.IsTest
				}
				codeRefs = append(codeRefs, specs.CodeRef{Path: r.Path, IsTest: isTest})
			}
			refsData[acid] = codeRefs
		}
		req.References = &services.RefsInput{
			Override: override,
			Data:     refsData,
		}
	}

	result, err := s.push.Execute(c.Request().Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrTooLarge):
			return apierror.WriteAppErrorEcho(c, http.StatusRequestEntityTooLarge, err.Error(), "")
		case errors.Is(err, services.ErrInvalidRequest):
			return apierror.WriteAppErrorEcho(c, http.StatusUnprocessableEntity, err.Error(), "")
		case errors.Is(err, services.ErrProductNotFound):
			return apierror.WriteAppErrorEcho(c, http.StatusUnprocessableEntity, "product not found", "")
		case errors.Is(err, services.ErrImplementationNotFound):
			return apierror.WriteAppErrorEcho(c, http.StatusUnprocessableEntity, "implementation not found", "")
		default:
			return apierror.WriteAppErrorEcho(c, http.StatusInternalServerError, "push failed", "")
		}
	}

	return c.JSON(http.StatusOK, buildPushResponse(result))
}

// pushResponseData mirrors spec.PushResponse.Data as a named type.
type pushResponseData struct {
	BranchID           string   `json:"branch_id"`
	ImplementationID   *string  `json:"implementation_id"`
	ImplementationName *string  `json:"implementation_name"`
	ProductName        *string  `json:"product_name"`
	SpecsCreated       int      `json:"specs_created"`
	SpecsUpdated       int      `json:"specs_updated"`
	Warnings           []string `json:"warnings"`
}

type pushResponse struct {
	Data pushResponseData `json:"data"`
}

func buildPushResponse(r *services.PushResult) pushResponse {
	data := pushResponseData{
		BranchID:     r.Branch.ID,
		SpecsCreated: r.SpecsCreated,
		SpecsUpdated: r.SpecsUpdated,
		Warnings:     r.Warnings,
	}
	if r.Implementation != nil {
		data.ImplementationID = &r.Implementation.ID
		data.ImplementationName = &r.Implementation.Name
	}
	if r.Product != nil {
		data.ProductName = &r.Product.Name
	}
	return pushResponse{Data: data}
}
