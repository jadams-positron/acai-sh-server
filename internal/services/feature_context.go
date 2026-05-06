// Package services contains cross-context business logic that orchestrates
// multiple domain repos. P2b-2 lands feature-context resolution; P2c will add
// push and feature-states services.
package services

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	"github.com/jadams-positron/acai-sh-server/internal/domain/specs"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
)

// FeatureContextService resolves the canonical feature view for the API.
type FeatureContextService struct {
	products *products.Repository
	impls    *implementations.Repository
	specs    *specs.Repository
}

// NewFeatureContextService constructs the service with required repos.
func NewFeatureContextService(p *products.Repository, i *implementations.Repository, s *specs.Repository) *FeatureContextService {
	return &FeatureContextService{products: p, impls: i, specs: s}
}

// FeatureContextRequest captures the inputs for Resolve.
type FeatureContextRequest struct {
	Team                  *teams.Team
	ProductName           string
	FeatureName           string
	ImplementationName    string
	IncludeRefs           bool
	IncludeDanglingStates bool
	IncludeDeprecated     bool
	StatusFilter          []string // empty = no filter; "null" sentinel = include null status
}

// FeatureContext is the resolved view ready for the response builder.
type FeatureContext struct {
	Product           *products.Product
	Implementation    *implementations.Implementation
	BranchForSpec     *specs.Branch // may be nil if no spec found
	BranchForRefs     *specs.Branch // may be nil if no refs found
	Spec              *specs.Spec
	Refs              *specs.FeatureBranchRef
	States            *specs.FeatureImplState
	IncludeRefs       bool
	IncludeDangling   bool
	IncludeDeprecated bool
	StatusFilter      []string
}

// ErrProductNotFound and ErrImplementationNotFound are sentinel errors callers
// may handle for 404 responses.
var (
	ErrProductNotFound        = errors.New("services: product not found")
	ErrImplementationNotFound = errors.New("services: implementation not found")
)

// Resolve performs the direct lookup: product → impl → branch → spec/refs/states.
// No inheritance walk. Returns ErrProductNotFound or ErrImplementationNotFound
// for 404 cases. Spec/refs/states may individually be nil — caller decides.
func (s *FeatureContextService) Resolve(ctx context.Context, req FeatureContextRequest) (*FeatureContext, error) {
	prod, err := s.products.GetByTeamAndName(ctx, req.Team.ID, req.ProductName)
	if err != nil {
		if products.IsNotFound(err) {
			return nil, ErrProductNotFound
		}
		return nil, err
	}

	impl, err := s.impls.GetByProductAndName(ctx, prod.ID, req.ImplementationName)
	if err != nil {
		if implementations.IsNotFound(err) {
			return nil, ErrImplementationNotFound
		}
		return nil, err
	}

	out := &FeatureContext{
		Product:           prod,
		Implementation:    impl,
		IncludeRefs:       req.IncludeRefs,
		IncludeDangling:   req.IncludeDanglingStates,
		IncludeDeprecated: req.IncludeDeprecated,
		StatusFilter:      req.StatusFilter,
	}

	// Pick the branch for refs (most-recent push for this feature).
	if branch, err := s.specs.PickRefsBranch(ctx, impl.ID, req.FeatureName); err == nil {
		out.BranchForRefs = branch
		if refs, err := s.specs.GetRefs(ctx, branch.ID, req.FeatureName); err == nil {
			out.Refs = refs
		}
	}

	// Pick the branch for spec — same branch as refs if present, else the first
	// tracked branch.
	branchForSpec := out.BranchForRefs
	if branchForSpec == nil {
		if b, err := s.specs.FirstTrackedBranch(ctx, impl.ID); err == nil {
			branchForSpec = b
		}
	}
	out.BranchForSpec = branchForSpec
	if branchForSpec != nil {
		if spec, err := s.specs.GetSpec(ctx, branchForSpec.ID, req.FeatureName); err == nil {
			out.Spec = spec
		}
	}

	if states, err := s.specs.GetStates(ctx, impl.ID, req.FeatureName); err == nil {
		out.States = states
	}

	return out, nil
}

// MatchesStatusFilter reports whether the given status (nil-or-string) passes
// req.StatusFilter. Empty filter means accept all. The sentinel string "null"
// in the filter matches a nil status.
func MatchesStatusFilter(filter []string, status *string) bool {
	if len(filter) == 0 {
		return true
	}
	want := make(map[string]struct{}, len(filter))
	for _, f := range filter {
		want[strings.ToLower(strings.TrimSpace(f))] = struct{}{}
	}
	if status == nil {
		_, ok := want["null"]
		return ok
	}
	_, ok := want[strings.ToLower(*status)]
	return ok
}

// SortedACIDs returns the requirements keys from spec sorted alphabetically.
// Phoenix returns ACIDs in deterministic order; we match that.
func SortedACIDs(reqs map[string]specs.Requirement) []string {
	out := make([]string, 0, len(reqs))
	for k := range reqs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
