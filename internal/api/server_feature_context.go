package api

import (
	"errors"
	"net/http"
	"sort"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/api/apierror"
	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/api/spec"
	"github.com/jadams-positron/acai-sh-server/internal/domain/specs"
	"github.com/jadams-positron/acai-sh-server/internal/services"
)

// AcaiWebApiFeatureContextControllerShow implements GET /api/v1/feature-context.
//
//nolint:revive,staticcheck // method name is dictated by oapi-codegen ServerInterface
func (s *Server) AcaiWebApiFeatureContextControllerShow(c echo.Context, params spec.AcaiWebApiFeatureContextControllerShowParams) error {
	team := middleware.TeamFromEcho(c)
	if team == nil {
		return apierror.WriteAppErrorEcho(c, http.StatusUnauthorized, "missing team context", "")
	}

	req := services.FeatureContextRequest{
		Team:               team,
		ProductName:        params.ProductName,
		FeatureName:        params.FeatureName,
		ImplementationName: params.ImplementationName,
	}
	if params.IncludeRefs != nil {
		req.IncludeRefs = *params.IncludeRefs
	}
	if params.IncludeDanglingStates != nil {
		req.IncludeDanglingStates = *params.IncludeDanglingStates
	}
	if params.IncludeDeprecated != nil {
		req.IncludeDeprecated = *params.IncludeDeprecated
	}
	if params.Statuses != nil {
		req.StatusFilter = *params.Statuses
	}

	fc, err := s.featureContext.Resolve(c.Request().Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrProductNotFound):
			return apierror.WriteAppErrorEcho(c, http.StatusNotFound, "product not found", "")
		case errors.Is(err, services.ErrImplementationNotFound):
			return apierror.WriteAppErrorEcho(c, http.StatusNotFound, "implementation not found", "")
		default:
			return apierror.WriteAppErrorEcho(c, http.StatusInternalServerError, "failed to resolve feature context", "")
		}
	}

	return c.JSON(http.StatusOK, buildFeatureContextResponse(req, fc))
}

// featureContextData mirrors spec.FeatureContextResponse.Data as a named type
// so we can build it with field-by-field assignment without fighting Go's
// anonymous-struct assignability rules. We return it as a map to avoid any
// type-mismatch issues when assigning to the generated response struct.
//
// Fields match the JSON tags on spec.FeatureContextResponse.Data exactly.
type featureContextData struct {
	Acids              []acidEntryJSON      `json:"acids"`
	DanglingStates     *[]danglingEntryJSON `json:"dangling_states,omitempty"`
	FeatureName        string               `json:"feature_name"`
	ImplementationId   string               `json:"implementation_id"` //nolint:revive,staticcheck // ST1003/var-naming: matches generated JSON tag
	ImplementationName string               `json:"implementation_name"`
	ProductName        string               `json:"product_name"`
	RefsSource         sourceJSON           `json:"refs_source"`
	SpecSource         sourceJSON           `json:"spec_source"`
	StatesSource       sourceJSON           `json:"states_source"`
	Summary            summaryJSON          `json:"summary"`
	Warnings           []string             `json:"warnings"`
}

type acidEntryJSON struct {
	Acid          string     `json:"acid"`
	Deprecated    *bool      `json:"deprecated,omitempty"`
	Note          *string    `json:"note,omitempty"`
	Refs          *[]refJSON `json:"refs,omitempty"`
	RefsCount     int        `json:"refs_count"`
	ReplacedBy    *[]string  `json:"replaced_by,omitempty"`
	Requirement   string     `json:"requirement"`
	State         stateJSON  `json:"state"`
	TestRefsCount int        `json:"test_refs_count"`
}

type refJSON struct {
	BranchName string `json:"branch_name"`
	IsTest     bool   `json:"is_test"`
	Path       string `json:"path"`
	RepoUri    string `json:"repo_uri"` //nolint:revive,staticcheck // ST1003/var-naming: matches generated JSON tag
}

type stateJSON struct {
	Comment   *string `json:"comment,omitempty"`
	Status    *string `json:"status"`
	UpdatedAt *string `json:"updated_at,omitempty"`
}

type danglingEntryJSON struct {
	Acid  string    `json:"acid"`
	State stateJSON `json:"state"`
}

type sourceJSON struct {
	BranchNames        *[]string `json:"branch_names,omitempty"`
	ImplementationName *string   `json:"implementation_name,omitempty"`
	SourceType         string    `json:"source_type"`
}

type summaryJSON struct {
	StatusCounts map[string]any `json:"status_counts"`
	TotalAcids   int            `json:"total_acids"`
}

// featureContextResponse wraps featureContextData so json.Marshal emits {"data":{...}}.
type featureContextResponse struct {
	Data featureContextData `json:"data"`
}

// buildFeatureContextResponse composes the response from the resolved FeatureContext.
func buildFeatureContextResponse(req services.FeatureContextRequest, fc *services.FeatureContext) featureContextResponse {
	data := featureContextData{
		FeatureName:        req.FeatureName,
		ProductName:        fc.Product.Name,
		ImplementationId:   fc.Implementation.ID,
		ImplementationName: fc.Implementation.Name,
		Warnings:           []string{},
	}

	implName := fc.Implementation.Name

	// Source fields — always "local" in P2b-2 (no inheritance).
	if fc.Spec != nil && fc.BranchForSpec != nil {
		branchName := fc.BranchForSpec.BranchName
		data.SpecSource = sourceJSON{
			SourceType:         "local",
			ImplementationName: &implName,
			BranchNames:        &[]string{branchName},
		}
	} else {
		data.SpecSource = sourceJSON{SourceType: "none"}
	}

	if fc.Refs != nil && fc.BranchForRefs != nil {
		branchName := fc.BranchForRefs.BranchName
		data.RefsSource = sourceJSON{
			SourceType:         "local",
			ImplementationName: &implName,
			BranchNames:        &[]string{branchName},
		}
	} else {
		data.RefsSource = sourceJSON{SourceType: "none"}
	}

	if fc.States != nil {
		data.StatesSource = sourceJSON{
			SourceType:         "local",
			ImplementationName: &implName,
		}
	} else {
		data.StatesSource = sourceJSON{SourceType: "none"}
	}

	// Build ACID entries and status counts.
	acids, statusCounts := buildAcidEntries(fc)
	data.Acids = acids
	data.Summary = summaryJSON{
		StatusCounts: statusCounts,
		TotalAcids:   len(acids),
	}

	// Dangling states.
	if fc.IncludeDangling && fc.States != nil {
		dangling := buildDanglingStates(fc)
		if dangling != nil {
			data.DanglingStates = &dangling
		}
	}

	return featureContextResponse{Data: data}
}

// buildAcidEntries walks fc.Spec.Requirements (sorted) and applies refs/states.
func buildAcidEntries(fc *services.FeatureContext) (entries []acidEntryJSON, statusCounts map[string]any) {
	statusCounts = map[string]any{}

	if fc.Spec == nil {
		return []acidEntryJSON{}, statusCounts
	}

	// Resolve refs map and branch info.
	var refsMap map[string][]specs.CodeRef
	var repoURI, branchName string
	if fc.Refs != nil {
		refsMap = fc.Refs.Refs
	}
	if fc.BranchForRefs != nil {
		repoURI = fc.BranchForRefs.RepoURI
		branchName = fc.BranchForRefs.BranchName
	}

	var statesMap map[string]specs.ACIDState
	if fc.States != nil {
		statesMap = fc.States.States
	}

	acids := services.SortedACIDs(fc.Spec.Requirements)
	out := make([]acidEntryJSON, 0, len(acids))

	for _, acid := range acids {
		req := fc.Spec.Requirements[acid]

		// Filter: deprecated.
		if !fc.IncludeDeprecated && req.Deprecated {
			continue
		}

		// Resolve state for this ACID.
		var state specs.ACIDState
		if statesMap != nil {
			state = statesMap[acid]
		}

		// Apply status filter.
		if !services.MatchesStatusFilter(fc.StatusFilter, state.Status) {
			continue
		}

		// Count status for summary.
		statusKey := "null"
		if state.Status != nil {
			statusKey = *state.Status
		}
		if cur, ok := statusCounts[statusKey].(int); ok {
			statusCounts[statusKey] = cur + 1
		} else {
			statusCounts[statusKey] = 1
		}

		// Build refs list for this ACID.
		var refsSlice *[]refJSON
		refsCount := 0
		testRefsCount := 0
		if refsMap != nil {
			if codeRefs, ok := refsMap[acid]; ok && len(codeRefs) > 0 {
				refs := make([]refJSON, 0, len(codeRefs))
				for _, cr := range codeRefs {
					refs = append(refs, refJSON{
						BranchName: branchName,
						IsTest:     cr.IsTest,
						Path:       cr.Path,
						RepoUri:    repoURI,
					})
					if cr.IsTest {
						testRefsCount++
					} else {
						refsCount++
					}
				}
				refsSlice = &refs
			}
		}

		entry := acidEntryJSON{
			Acid:          acid,
			Requirement:   req.Requirement,
			RefsCount:     refsCount,
			TestRefsCount: testRefsCount,
		}

		if req.Deprecated {
			dep := true
			entry.Deprecated = &dep
		}
		if req.Note != nil {
			entry.Note = req.Note
		}
		if len(req.ReplacedBy) > 0 {
			rb := req.ReplacedBy
			entry.ReplacedBy = &rb
		}
		if refsSlice != nil {
			entry.Refs = refsSlice
		}

		// State.
		entry.State.Status = state.Status
		entry.State.Comment = state.Comment
		if state.UpdatedAt != nil {
			ts := state.UpdatedAt.Format("2006-01-02T15:04:05Z")
			entry.State.UpdatedAt = &ts
		}

		out = append(out, entry)
	}

	if out == nil {
		out = []acidEntryJSON{}
	}

	return out, statusCounts
}

// buildDanglingStates returns states whose ACID is NOT in the spec requirements.
func buildDanglingStates(fc *services.FeatureContext) []danglingEntryJSON {
	if fc.States == nil {
		return nil
	}

	// Gather spec ACIDs for quick lookup.
	inSpec := make(map[string]struct{})
	if fc.Spec != nil {
		for acid := range fc.Spec.Requirements {
			inSpec[acid] = struct{}{}
		}
	}

	// Collect dangling ACIDs.
	var dangling []string
	for acid := range fc.States.States {
		if _, ok := inSpec[acid]; !ok {
			dangling = append(dangling, acid)
		}
	}
	if len(dangling) == 0 {
		return nil
	}

	sort.Strings(dangling)

	out := make([]danglingEntryJSON, 0, len(dangling))
	for _, acid := range dangling {
		st := fc.States.States[acid]
		entry := danglingEntryJSON{Acid: acid}
		entry.State.Status = st.Status
		entry.State.Comment = st.Comment
		if st.UpdatedAt != nil {
			ts := st.UpdatedAt.Format("2006-01-02T15:04:05Z")
			entry.State.UpdatedAt = &ts
		}
		out = append(out, entry)
	}

	return out
}
