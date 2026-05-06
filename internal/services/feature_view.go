// Package services — adds FeatureViewService that composes the data needed
// for the /t/{team}/f/{feature} page and the /t/{team}/i/{impl}/f/{feature} page.
package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

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

// ImplFeatureSummary is one row in an impl-overview table — name, total
// ACIDs from the spec, and the per-status fold computed by buildStatusCounts.
//
// Distinct from services.FeatureSummary which is the existing
// /api/v1/implementation-features wire shape (test/ref counts, sources, etc.).
type ImplFeatureSummary struct {
	FeatureName       string
	TotalRequirements int
	Counts            StatusCounts
}

// ImplOverview composes the per-feature breakdown plus the team-wide
// aggregate for one implementation.
type ImplOverview struct {
	Features        []*ImplFeatureSummary
	AggregateTotal  int
	AggregateCounts StatusCounts
}

// ImplOverviewRequest is the input to ResolveImplOverview.
type ImplOverviewRequest struct {
	Implementation *implementations.Implementation
}

// ResolveImplOverview returns the per-feature progress breakdown for one
// implementation, plus an aggregate fold across all features. Features that
// have no spec on this impl's tracked branches are skipped — they don't
// belong to this impl's lifecycle.
//
// Cost: 1 query for branches + N queries for spec lookups + 1 batched
// states query + per-feature in-memory fold. Acceptable at typical impl
// sizes (handful of features); a single batched JOIN is the obvious next
// move if feature counts grow.
func (s *FeatureViewService) ResolveImplOverview(ctx context.Context, req ImplOverviewRequest) (*ImplOverview, error) {
	impl := req.Implementation
	if impl == nil {
		return nil, fmt.Errorf("services: ResolveImplOverview: implementation is required")
	}

	branch, err := s.specs.FirstTrackedBranch(ctx, impl.ID)
	if err != nil {
		// No tracked branches means no specs / no progress to report.
		if errors.Is(err, specs.ErrNotFound) {
			return &ImplOverview{}, nil
		}
		return nil, fmt.Errorf("services: ResolveImplOverview: branch lookup: %w", err)
	}

	specsByName, err := s.specs.ListSpecsForBranch(ctx, branch.ID)
	if err != nil {
		return nil, fmt.Errorf("services: ResolveImplOverview: list specs: %w", err)
	}

	out := &ImplOverview{}
	for _, sp := range specsByName {
		states, _ := s.specs.GetStates(ctx, impl.ID, sp.FeatureName)
		total := len(sp.Requirements)
		counts := buildStatusCounts(states, total)
		out.Features = append(out.Features, &ImplFeatureSummary{
			FeatureName:       sp.FeatureName,
			TotalRequirements: total,
			Counts:            counts,
		})
		out.AggregateTotal += total
		out.AggregateCounts.Null += counts.Null
		out.AggregateCounts.Assigned += counts.Assigned
		out.AggregateCounts.Blocked += counts.Blocked
		out.AggregateCounts.Incomplete += counts.Incomplete
		out.AggregateCounts.Completed += counts.Completed
		out.AggregateCounts.Rejected += counts.Rejected
		out.AggregateCounts.Accepted += counts.Accepted
	}
	sort.Slice(out.Features, func(i, j int) bool {
		return out.Features[i].FeatureName < out.Features[j].FeatureName
	})
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

// ParseImplSlug returns the impl ID from a slug of the form {name}-{uuidNoDashes}.
// Returns "" if the slug doesn't match the expected shape.
func ParseImplSlug(slug string) string {
	// A UUID-no-dashes is exactly 32 hex chars. The slug ends with a '-' before
	// that 32-char segment. Find the last '-', validate the suffix is 32 hex chars,
	// then reinsert dashes in the 8-4-4-4-12 layout.
	idx := -1
	for i := len(slug) - 1; i >= 0; i-- {
		if slug[i] == '-' {
			idx = i
			break
		}
	}
	if idx < 0 || idx == len(slug)-1 {
		return ""
	}
	rest := slug[idx+1:]
	if len(rest) != 32 {
		return ""
	}
	for i := range len(rest) {
		c := rest[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return ""
		}
	}
	// Reinsert dashes: 8-4-4-4-12 layout.
	return rest[:8] + "-" + rest[8:12] + "-" + rest[12:16] + "-" + rest[16:20] + "-" + rest[20:]
}

// ImplFeatureView is the resolved data for the impl×feature drill-down page.
type ImplFeatureView struct {
	Implementation *implementations.Implementation
	Spec           *specs.Spec
	Refs           *specs.FeatureBranchRef
	States         *specs.FeatureImplState
	AcidEntries    []*ACIDEntry
}

// ACIDEntry is one ACID row with its derived state and refs.
type ACIDEntry struct {
	ACID          string
	Requirement   string
	Deprecated    bool
	Status        *string
	Comment       *string
	UpdatedAt     *time.Time
	Refs          []specs.CodeRef
	RefsCount     int // count of non-test refs
	TestRefsCount int // count of test refs
}

// ImplFeatureViewRequest is the input for ResolveImplFeatureView.
type ImplFeatureViewRequest struct {
	Team              *teams.Team
	ImplementationID  string
	FeatureName       string
	IncludeDeprecated bool
}

// ResolveImplFeatureView composes the page data for the impl×feature drill-down.
func (s *FeatureViewService) ResolveImplFeatureView(ctx context.Context, req ImplFeatureViewRequest) (*ImplFeatureView, error) {
	impl, err := s.impls.GetByID(ctx, req.ImplementationID, req.Team.ID)
	if err != nil {
		return nil, err
	}

	out := &ImplFeatureView{Implementation: impl}

	// Pick the refs branch (most-recent pushed_at for this feature), then fall
	// back to the first tracked branch for the spec.
	var branch *specs.Branch
	if b, berr := s.specs.PickRefsBranch(ctx, impl.ID, req.FeatureName); berr == nil {
		branch = b
		if refs, rerr := s.specs.GetRefs(ctx, branch.ID, req.FeatureName); rerr == nil {
			out.Refs = refs
		}
	}
	if branch == nil {
		if b, berr := s.specs.FirstTrackedBranch(ctx, impl.ID); berr == nil {
			branch = b
		}
	}
	if branch != nil {
		if spec, serr := s.specs.GetSpec(ctx, branch.ID, req.FeatureName); serr == nil {
			out.Spec = spec
		}
	}
	if states, serr := s.specs.GetStates(ctx, impl.ID, req.FeatureName); serr == nil {
		out.States = states
	}

	if out.Spec != nil {
		out.AcidEntries = buildACIDEntries(out.Spec, out.Refs, out.States, req.IncludeDeprecated)
	}
	return out, nil
}

func buildACIDEntries(spec *specs.Spec, refs *specs.FeatureBranchRef, states *specs.FeatureImplState, includeDeprecated bool) []*ACIDEntry {
	keys := make([]string, 0, len(spec.Requirements))
	for k := range spec.Requirements {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []*ACIDEntry
	for _, acid := range keys {
		r := spec.Requirements[acid]
		if r.Deprecated && !includeDeprecated {
			continue
		}
		e := &ACIDEntry{
			ACID:        acid,
			Requirement: r.Requirement,
			Deprecated:  r.Deprecated,
		}
		if states != nil {
			if st, ok := states.States[acid]; ok {
				e.Status = st.Status
				e.Comment = st.Comment
				e.UpdatedAt = st.UpdatedAt
			}
		}
		if refs != nil {
			if refList, ok := refs.Refs[acid]; ok {
				e.Refs = refList
				for _, rr := range refList {
					if rr.IsTest {
						e.TestRefsCount++
					} else {
						e.RefsCount++
					}
				}
			}
		}
		out = append(out, e)
	}
	return out
}
