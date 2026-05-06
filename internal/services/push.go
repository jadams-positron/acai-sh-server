package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	"github.com/jadams-positron/acai-sh-server/internal/domain/specs"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
)

var commitHashRE = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

func isValidCommitHash(s string) bool {
	return commitHashRE.MatchString(s)
}

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

	// --- Validate commit_hash format ---
	if !isValidCommitHash(req.CommitHash) {
		return nil, fmt.Errorf("%w: commit_hash must be a 7-40 character hex string", ErrInvalidRequest)
	}

	// --- 1. Upsert branch ---
	branch, _, err := s.specs.UpsertBranch(ctx, req.Team.ID, req.RepoURI, req.BranchName, req.CommitHash)
	if err != nil {
		return nil, err
	}

	result := &PushResult{Branch: branch, Warnings: []string{}}

	// --- 2. Process specs and resolve the request product ---
	// push.NEW_IMPLS.4: a single push targets exactly one product.
	// push.VALIDATION.6: when both specs and product_name are present they must agree.
	requestProduct, err := s.processSpecsAndResolveProduct(ctx, req, branch, result)
	if err != nil {
		return nil, err
	}
	if requestProduct != nil {
		result.Product = requestProduct
	}

	// --- 3. Resolve implementation context ---
	// Only when we have something to do for an impl: either specs (NEW_IMPLS.1)
	// or refs (LINK_IMPLS / EXISTING_IMPLS / inference). Refs-only without any
	// impl signals is allowed to leave the branch untracked (REFS.7).
	if len(req.Specs) > 0 || req.References != nil {
		impl, prod, err := s.resolveImpl(ctx, req, branch, requestProduct, len(req.Specs) > 0)
		if err != nil {
			return nil, err
		}
		if impl != nil {
			result.Implementation = impl
			if prod != nil {
				result.Product = prod
			}
			// Track branch ↔ impl (idempotent — push.IDEMPOTENCY.1).
			if err := s.specs.UpsertTrackedBranch(ctx, impl.ID, branch.ID, req.RepoURI); err != nil {
				return nil, err
			}
		}
	}

	// --- 4. Process references ---
	if req.References != nil {
		byFeature := groupRefsByFeature(req.References.Data)
		for featureName, featureRefs := range byFeature {
			if err := s.specs.UpsertFeatureBranchRef(ctx, branch.ID, featureName, featureRefs, req.CommitHash, req.References.Override); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}

// processSpecsAndResolveProduct upserts each spec, creating products on demand,
// while validating push.NEW_IMPLS.4 (single product per push) and
// push.VALIDATION.6 (specs' product matches product_name when both given).
// Returns the resolved request product (or nil for refs-only without product_name).
func (s *PushService) processSpecsAndResolveProduct(
	ctx context.Context, req PushRequest, branch *specs.Branch, result *PushResult,
) (*products.Product, error) {
	var requestProduct *products.Product

	for _, sp := range req.Specs {
		prod, err := s.products.GetOrCreate(ctx, req.Team.ID, sp.FeatureProduct)
		if err != nil {
			return nil, err
		}

		if requestProduct == nil {
			requestProduct = prod
		} else if requestProduct.ID != prod.ID {
			// push.NEW_IMPLS.4
			return nil, fmt.Errorf("%w: specs span multiple products (%q and %q); split into separate pushes",
				ErrInvalidRequest, requestProduct.Name, prod.Name)
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

	// push.VALIDATION.6: specs and product_name must agree when both present.
	if requestProduct != nil && req.ProductName != nil && *req.ProductName != requestProduct.Name {
		return nil, fmt.Errorf("%w: product_name %q does not match specs' feature.product %q",
			ErrInvalidRequest, *req.ProductName, requestProduct.Name)
	}

	// Refs-only path: look up the explicit product when no specs supplied it.
	if requestProduct == nil && req.ProductName != nil {
		p, err := s.products.GetByTeamAndName(ctx, req.Team.ID, *req.ProductName)
		if err != nil {
			if products.IsNotFound(err) {
				return nil, ErrProductNotFound
			}
			return nil, err
		}
		requestProduct = p
	}

	return requestProduct, nil
}

// resolveImpl picks (or creates) the implementation context for this push,
// following push.feature.yaml semantics:
//
//   - push.EXISTING_IMPLS.1 / .2 / .3 / .4: when the branch is already tracked
//   - push.NEW_IMPLS.1 / .1-1: auto-create on specs push to untracked branch
//   - push.NEW_IMPLS.6: refs-only child impl creation
//   - push.LINK_IMPLS.1 / .5: link untracked branch to existing impl
//   - push.PARENTS.1 / .3: parent inheritance
//
// product may be nil for refs-only pushes without product context. In that
// case only inference from tracked_branches applies. Returns (impl, product,
// error). product is the impl's product (it may be inferred from the impl
// when the request did not carry one). A nil impl with nil error means
// "no impl context" — refs may still be written without linkage (REFS.7).
//
//nolint:gocognit,gocyclo,cyclop // single-purpose decision tree, factoring would obscure the spec mapping
func (s *PushService) resolveImpl(
	ctx context.Context,
	req PushRequest,
	branch *specs.Branch,
	product *products.Product,
	hasSpecs bool,
) (*implementations.Implementation, *products.Product, error) {
	tracked, err := s.impls.ListTrackingBranch(ctx, req.Team.ID, branch.ID)
	if err != nil {
		return nil, nil, err
	}

	// === Branch already tracked ===
	if len(tracked) > 0 {
		// When we know the request product, restrict candidates to it; otherwise
		// fall back to the cross-product set (the inference path).
		candidates := tracked
		if product != nil {
			filtered := make([]*implementations.Implementation, 0, len(tracked))
			for _, t := range tracked {
				if t.ProductID == product.ID {
					filtered = append(filtered, t)
				}
			}
			candidates = filtered
		}

		switch len(candidates) {
		case 0:
			// Branch tracked by impls in other products only. Treat as
			// "untracked in this product" and fall through to creation/linking.
		case 1:
			impl := candidates[0]
			// push.EXISTING_IMPLS.4
			if req.TargetImplName != nil && *req.TargetImplName != impl.Name {
				return nil, nil, fmt.Errorf("%w: branch is already tracked by implementation %q (cannot retarget to %q); to re-link, use the frontend",
					ErrInvalidRequest, impl.Name, *req.TargetImplName)
			}
			p, err := s.resolveProductForImpl(ctx, req.Team.ID, product, impl)
			if err != nil {
				return nil, nil, err
			}
			return impl, p, nil
		default:
			// push.EXISTING_IMPLS.2 / .3
			if req.TargetImplName == nil {
				names := make([]string, 0, len(candidates))
				for _, t := range candidates {
					names = append(names, t.Name)
				}
				return nil, nil, fmt.Errorf("%w: branch is tracked by multiple implementations (%s); provide target_impl_name",
					ErrInvalidRequest, strings.Join(names, ", "))
			}
			for _, t := range candidates {
				if t.Name == *req.TargetImplName {
					p, err := s.resolveProductForImpl(ctx, req.Team.ID, product, t)
					if err != nil {
						return nil, nil, err
					}
					return t, p, nil
				}
			}
			return nil, nil, fmt.Errorf("%w: target_impl_name %q not found among tracking implementations",
				ErrInvalidRequest, *req.TargetImplName)
		}
	}

	// === Branch not tracked (in this product) ===

	// Without a product context we cannot create or look up impls; the
	// inference path above already handled the cross-product case.
	if product == nil {
		return nil, nil, nil
	}

	// push.NEW_IMPLS.1-1: implementation name = target_impl_name OR branch_name.
	implName := req.BranchName
	if req.TargetImplName != nil {
		implName = *req.TargetImplName
	}

	existing, err := s.impls.GetByProductAndName(ctx, product.ID, implName)
	if err != nil && !implementations.IsNotFound(err) {
		return nil, nil, err
	}

	if existing != nil {
		// push.LINK_IMPLS.4: parent_impl_name + same-name impl → never links.
		// Treat as a name collision (push.NEW_IMPLS.5 / push.LINK_IMPLS.5).
		if req.ParentImplName != nil {
			return nil, nil, fmt.Errorf("%w: implementation %q already exists in product %q; cannot create child with the same name (provide a different target_impl_name)",
				ErrInvalidRequest, implName, product.Name)
		}
		// push.LINK_IMPLS.1: link untracked branch to existing impl.
		// (NOTE: the LINK_IMPLS.1.4 condition — "that impl does not already
		// track a branch in this repo_uri" — is intentionally not enforced
		// here; for MVP we link via UpsertTrackedBranch idempotently. A
		// stricter check can be added later without breaking callers.)
		return existing, product, nil
	}

	// No existing impl with this name in product.

	if !hasSpecs {
		// Refs-only path: creation requires explicit target_impl_name AND
		// parent_impl_name (push.NEW_IMPLS.6 / push.LINK_IMPLS.5).
		if req.ParentImplName == nil {
			if req.TargetImplName != nil {
				return nil, nil, fmt.Errorf("%w: implementation %q not found in product %q (and no parent_impl_name to create child from)",
					ErrInvalidRequest, implName, product.Name)
			}
			// Refs-only without any impl signals: leave branch untracked (REFS.7).
			return nil, product, nil
		}
		// Refs-only + parent_impl_name → creation falls through.
	}

	// push.NEW_IMPLS.1 (specs) or push.NEW_IMPLS.6 (refs-only with parent).
	var parentID *string
	if req.ParentImplName != nil {
		parent, err := s.impls.GetByProductAndName(ctx, product.ID, *req.ParentImplName)
		if err != nil {
			if implementations.IsNotFound(err) {
				// push.PARENTS.3
				return nil, nil, fmt.Errorf("%w: parent_impl_name %q not found in product %q",
					ErrInvalidRequest, *req.ParentImplName, product.Name)
			}
			return nil, nil, err
		}
		parentID = &parent.ID
	}

	newImpl, err := s.impls.Create(ctx, implementations.CreateImplementationParams{
		ProductID:              product.ID,
		TeamID:                 req.Team.ID,
		Name:                   implName,
		ParentImplementationID: parentID,
	})
	if err != nil {
		return nil, nil, err
	}
	return newImpl, product, nil
}

// resolveProductForImpl returns product if non-nil, otherwise looks up the
// impl's product by name. Used when the inference path needs to fill in the
// product after picking an impl from tracked_branches.
func (s *PushService) resolveProductForImpl(
	ctx context.Context, teamID string, product *products.Product, impl *implementations.Implementation,
) (*products.Product, error) {
	if product != nil {
		return product, nil
	}
	return s.products.GetByTeamAndName(ctx, teamID, impl.ProductName)
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
