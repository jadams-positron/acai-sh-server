package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	"github.com/jadams-positron/acai-sh-server/internal/domain/specs"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
)

// PushService orchestrates the /api/v1/push write path.
type PushService struct {
	products *products.Repository
	impls    *implementations.Repository
	specs    *specs.Repository
}

// NewPushService constructs the service with required repos.
func NewPushService(p *products.Repository, i *implementations.Repository, s *specs.Repository) *PushService {
	return &PushService{products: p, impls: i, specs: s}
}

// PushRequest is the service-layer input (translated from the API spec type).
type PushRequest struct {
	Team           *teams.Team
	RepoURI        string
	BranchName     string
	CommitHash     string
	ProductName    *string // for refs-only impl resolution
	TargetImplName *string // for refs target resolution
	ParentImplName *string // ignored in v1
	Specs          []SpecInput
	References     *RefsInput

	// Semantic caps (0 = uncapped).
	MaxSpecs                    int
	MaxReferences               int
	MaxRequirementsPerSpec      int
	MaxRawContentBytes          int
	MaxRequirementStringLength  int
	MaxFeatureDescriptionLength int
	MaxMetaPathLength           int
	MaxRepoURILength            int
}

// SpecInput is one element of PushRequest.Specs.
type SpecInput struct {
	FeatureName        string
	FeatureProduct     string // the spec's feature.product field
	FeatureDescription *string
	FeatureVersion     string
	Path               string
	LastSeenCommit     string
	RawContent         *string
	Requirements       map[string]RequirementInput
}

// RequirementInput mirrors one entry in SpecInput.Requirements.
type RequirementInput struct {
	Requirement string
	Deprecated  bool
	Note        *string
	ReplacedBy  []string
}

// RefsInput carries the optional references payload.
type RefsInput struct {
	Override bool
	Data     map[string][]specs.CodeRef // ACID → list of refs
}

// PushResult is the service output.
type PushResult struct {
	Branch         *specs.Branch
	Implementation *implementations.Implementation // nil if no impl resolved
	Product        *products.Product               // nil if no product resolved
	SpecsCreated   int
	SpecsUpdated   int
	Warnings       []string
}

// Sentinel errors that the handler maps to HTTP status codes.
var (
	ErrTooLarge       = errors.New("services: payload too large")
	ErrInvalidRequest = errors.New("services: invalid request")
	// ErrProductNotFound and ErrImplementationNotFound are shared with feature_context.go.
)

// Execute runs the push business logic.
func (s *PushService) Execute(ctx context.Context, req PushRequest) (*PushResult, error) {
	// --- Semantic cap checks ---
	if req.MaxSpecs > 0 && len(req.Specs) > req.MaxSpecs {
		return nil, fmt.Errorf("%w: too many specs (%d > %d)", ErrTooLarge, len(req.Specs), req.MaxSpecs)
	}
	if req.References != nil && req.MaxReferences > 0 && len(req.References.Data) > req.MaxReferences {
		return nil, fmt.Errorf("%w: too many references (%d > %d)", ErrTooLarge, len(req.References.Data), req.MaxReferences)
	}
	if req.MaxRepoURILength > 0 && len(req.RepoURI) > req.MaxRepoURILength {
		return nil, fmt.Errorf("%w: repo_uri exceeds %d chars", ErrTooLarge, req.MaxRepoURILength)
	}
	for _, sp := range req.Specs {
		if req.MaxRequirementsPerSpec > 0 && len(sp.Requirements) > req.MaxRequirementsPerSpec {
			return nil, fmt.Errorf("%w: spec %q has %d requirements (max %d)", ErrTooLarge, sp.FeatureName, len(sp.Requirements), req.MaxRequirementsPerSpec)
		}
		if sp.RawContent != nil && req.MaxRawContentBytes > 0 && len(*sp.RawContent) > req.MaxRawContentBytes {
			return nil, fmt.Errorf("%w: spec %q raw_content exceeds %d bytes", ErrTooLarge, sp.FeatureName, req.MaxRawContentBytes)
		}
		if sp.FeatureDescription != nil && req.MaxFeatureDescriptionLength > 0 && len(*sp.FeatureDescription) > req.MaxFeatureDescriptionLength {
			return nil, fmt.Errorf("%w: spec %q description exceeds %d chars", ErrTooLarge, sp.FeatureName, req.MaxFeatureDescriptionLength)
		}
		if req.MaxMetaPathLength > 0 && len(sp.Path) > req.MaxMetaPathLength {
			return nil, fmt.Errorf("%w: spec %q path exceeds %d chars", ErrTooLarge, sp.FeatureName, req.MaxMetaPathLength)
		}
		for acid, r := range sp.Requirements {
			if req.MaxRequirementStringLength > 0 && len(r.Requirement) > req.MaxRequirementStringLength {
				return nil, fmt.Errorf("%w: requirement %s in spec %q exceeds %d chars", ErrTooLarge, acid, sp.FeatureName, req.MaxRequirementStringLength)
			}
		}
	}

	// --- 1. Upsert branch ---
	branch, _, err := s.specs.UpsertBranch(ctx, req.Team.ID, req.RepoURI, req.BranchName, req.CommitHash)
	if err != nil {
		return nil, err
	}

	result := &PushResult{Branch: branch, Warnings: []string{}}

	// --- 2. Process specs ---
	for _, sp := range req.Specs {
		prod, err := s.products.GetByTeamAndName(ctx, req.Team.ID, sp.FeatureProduct)
		if err != nil {
			if products.IsNotFound(err) {
				return nil, fmt.Errorf("%w: product %q not found (auto-create deferred to v2)", ErrInvalidRequest, sp.FeatureProduct)
			}
			return nil, err
		}
		if result.Product == nil {
			result.Product = prod
		}

		var pathPtr *string
		if sp.Path != "" {
			pathPtr = &sp.Path
		}

		_, created, err := s.specs.UpsertSpec(ctx, specs.UpsertSpecParams{
			ProductID:          prod.ID,
			BranchID:           branch.ID,
			Path:               pathPtr,
			LastSeenCommit:     sp.LastSeenCommit,
			FeatureName:        sp.FeatureName,
			FeatureDescription: sp.FeatureDescription,
			FeatureVersion:     sp.FeatureVersion,
			RawContent:         sp.RawContent,
			Requirements:       requirementsToMap(sp.Requirements),
		})
		if err != nil {
			return nil, err
		}
		if created {
			result.SpecsCreated++
		} else {
			result.SpecsUpdated++
		}
	}

	// --- 3. Process references ---
	if req.References != nil {
		if req.ProductName == nil {
			return nil, fmt.Errorf("%w: product_name is required for references", ErrInvalidRequest)
		}
		if req.TargetImplName == nil {
			return nil, fmt.Errorf("%w: target_impl_name is required for references (parent_impl_name auto-creation deferred to v2)", ErrInvalidRequest)
		}

		prod, err := s.products.GetByTeamAndName(ctx, req.Team.ID, *req.ProductName)
		if err != nil {
			if products.IsNotFound(err) {
				return nil, ErrProductNotFound
			}
			return nil, err
		}

		impl, err := s.impls.GetByProductAndName(ctx, prod.ID, *req.TargetImplName)
		if err != nil {
			if implementations.IsNotFound(err) {
				return nil, ErrImplementationNotFound
			}
			return nil, err
		}
		result.Product = prod
		result.Implementation = impl

		// Track branch ↔ impl.
		if err := s.specs.UpsertTrackedBranch(ctx, impl.ID, branch.ID, req.RepoURI); err != nil {
			return nil, err
		}

		// Group refs by feature name and upsert each feature row.
		byFeature := groupRefsByFeature(req.References.Data)
		for featureName, featureRefs := range byFeature {
			if err := s.specs.UpsertFeatureBranchRef(ctx, branch.ID, featureName, featureRefs, req.CommitHash, req.References.Override); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}

// requirementsToMap translates RequirementInput → map[string]specs.Requirement.
func requirementsToMap(reqs map[string]RequirementInput) map[string]specs.Requirement {
	out := make(map[string]specs.Requirement, len(reqs))
	for k, v := range reqs {
		out[k] = specs.Requirement{
			Requirement: v.Requirement,
			Deprecated:  v.Deprecated,
			Note:        v.Note,
			ReplacedBy:  v.ReplacedBy,
		}
	}
	return out
}

// groupRefsByFeature splits the flat ACID-keyed map into per-feature buckets.
// ACID format: "<feature_name>.<NAMESPACE>.<n>". The feature_name is everything
// before the first '.'.
func groupRefsByFeature(byACID map[string][]specs.CodeRef) map[string]map[string][]specs.CodeRef {
	out := map[string]map[string][]specs.CodeRef{}
	for acid, refs := range byACID {
		idx := -1
		for i, c := range acid {
			if c == '.' {
				idx = i
				break
			}
		}
		if idx < 1 {
			continue // skip malformed ACIDs (no '.' or starts with '.')
		}
		featureName := acid[:idx]
		if out[featureName] == nil {
			out[featureName] = map[string][]specs.CodeRef{}
		}
		out[featureName][acid] = refs
	}
	return out
}
