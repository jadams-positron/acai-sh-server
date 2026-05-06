// Package services — adds FeatureViewService that composes the data needed
// for the /t/{team}/f/{feature} page.
package services

import (
	"context"
	"sort"

	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	"github.com/jadams-positron/acai-sh-server/internal/domain/specs"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
)

// FeatureViewService composes the data for the /t/{team}/f/{feature} page.
type FeatureViewService struct {
	products *products.Repository
	impls    *implementations.Repository
	specs    *specs.Repository
}

// NewFeatureViewService constructs the service with required repos.
func NewFeatureViewService(p *products.Repository, i *implementations.Repository, s *specs.Repository) *FeatureViewService {
	return &FeatureViewService{products: p, impls: i, specs: s}
}

// FeatureViewRequest captures the inputs.
type FeatureViewRequest struct {
	Team        *teams.Team
	FeatureName string
}

// ImplementationCard is one impl summary on the feature page.
type ImplementationCard struct {
	ImplementationID       string
	ImplementationName     string
	ImplementationSlug     string // "{name}-{uuidwithoutdashes}"
	ProductName            string
	ParentImplementationID *string
	TotalRequirements      int
	Counts                 StatusCounts
}

// StatusCounts is the per-status count for an impl-feature.
type StatusCounts struct {
	Null       int
	Assigned   int
	Blocked    int
	Incomplete int
	Completed  int
	Rejected   int
	Accepted   int
}

// FeatureView is the resolved view ready for the response builder.
type FeatureView struct {
	FeatureName        string
	FeatureDescription string
	Cards              []*ImplementationCard
}

// Resolve composes the page data.
func (s *FeatureViewService) Resolve(ctx context.Context, req FeatureViewRequest) (*FeatureView, error) {
	// List all impls in the team.
	impls, err := s.impls.List(ctx, implementations.ListByTeamParams{TeamID: req.Team.ID})
	if err != nil {
		return nil, err
	}

	out := &FeatureView{FeatureName: req.FeatureName}

	// For each impl, compute the card. We use direct lookup (no inheritance walk
	// — same v1 pattern as /api/v1/feature-context).
	for _, impl := range impls {
		// Find the canonical spec for this feature on this impl. v1 strategy:
		// pick the first tracked branch and look up spec by (branch_id, feature).
		branch, ferr := s.specs.FirstTrackedBranch(ctx, impl.ID)
		var totalReqs int
		if ferr == nil {
			spec, serr := s.specs.GetSpec(ctx, branch.ID, req.FeatureName)
			if serr == nil {
				totalReqs = len(spec.Requirements)
				if out.FeatureDescription == "" && spec.FeatureDescription != nil {
					out.FeatureDescription = *spec.FeatureDescription
				}
			}
		}

		// Skip impls that don't have this feature at all (no spec, no states).
		states, _ := s.specs.GetStates(ctx, impl.ID, req.FeatureName)
		if totalReqs == 0 && states == nil {
			continue
		}

		counts := buildStatusCounts(states, totalReqs)

		out.Cards = append(out.Cards, &ImplementationCard{
			ImplementationID:       impl.ID,
			ImplementationName:     impl.Name,
			ImplementationSlug:     impl.Name + "-" + stripDashes(impl.ID),
			ProductName:            impl.ProductName,
			ParentImplementationID: impl.ParentImplementationID,
			TotalRequirements:      totalReqs,
			Counts:                 counts,
		})
	}

	out.Cards = orderByHierarchy(out.Cards)

	return out, nil
}

// buildStatusCounts walks the states map and counts statuses, treating any
// requirement without a state as "null".
func buildStatusCounts(states *specs.FeatureImplState, totalReqs int) StatusCounts {
	var c StatusCounts
	countedACIDs := 0
	if states != nil {
		for _, st := range states.States {
			countedACIDs++
			if st.Status == nil {
				c.Null++
				continue
			}
			switch *st.Status {
			case "assigned":
				c.Assigned++
			case "blocked":
				c.Blocked++
			case "incomplete":
				c.Incomplete++
			case "completed":
				c.Completed++
			case "rejected":
				c.Rejected++
			case "accepted":
				c.Accepted++
			default:
				c.Null++
			}
		}
	}
	// Any requirement without a state row gets counted as null.
	if totalReqs > countedACIDs {
		c.Null += totalReqs - countedACIDs
	}
	return c
}

// orderByHierarchy returns cards with each parent immediately followed by its
// children (depth-first). Cards whose parent isn't present get root-level
// placement.
func orderByHierarchy(cards []*ImplementationCard) []*ImplementationCard {
	if len(cards) == 0 {
		return cards
	}
	byID := make(map[string]*ImplementationCard, len(cards))
	children := make(map[string][]*ImplementationCard) // parent_id → []
	var roots []*ImplementationCard

	for _, c := range cards {
		byID[c.ImplementationID] = c
	}
	for _, c := range cards {
		if c.ParentImplementationID == nil {
			roots = append(roots, c)
			continue
		}
		if _, ok := byID[*c.ParentImplementationID]; ok {
			children[*c.ParentImplementationID] = append(children[*c.ParentImplementationID], c)
		} else {
			roots = append(roots, c) // parent not in set, treat as root
		}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].ImplementationName < roots[j].ImplementationName })
	for k := range children {
		sort.Slice(children[k], func(i, j int) bool {
			return children[k][i].ImplementationName < children[k][j].ImplementationName
		})
	}

	var out []*ImplementationCard
	var visit func(c *ImplementationCard)
	visit = func(c *ImplementationCard) {
		out = append(out, c)
		for _, ch := range children[c.ImplementationID] {
			visit(ch)
		}
	}
	for _, r := range roots {
		visit(r)
	}
	return out
}

// stripDashes removes all '-' chars from s. Used to build the impl slug.
func stripDashes(s string) string {
	out := make([]byte, 0, len(s))
	for i := range len(s) {
		if s[i] != '-' {
			out = append(out, s[i])
		}
	}
	return string(out)
}
