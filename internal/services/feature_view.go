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

// ProductImplSummary pairs an implementation with its per-feature progress
// breakdown. Used as one row in the product overview's Implementations table.
type ProductImplSummary struct {
	Implementation *implementations.Implementation
	Overview       *ImplOverview
}

// ProductFeatureSummary aggregates one feature's progress across all impls
// in a product. The TotalRequirements / Counts are the sum across impls,
// so a feature implemented in 3 impls × 5 ACIDs each shows TotalRequirements=15.
type ProductFeatureSummary struct {
	FeatureName       string
	ImplCount         int
	TotalRequirements int
	Counts            StatusCounts
}

// ProductOverview composes everything the product detail page needs:
// per-impl summaries, per-feature aggregates across the product's impls,
// and a top-line aggregate.
type ProductOverview struct {
	Impls           []*ProductImplSummary
	Features        []*ProductFeatureSummary
	AggregateTotal  int
	AggregateCounts StatusCounts
}

// ProductOverviewRequest is the input to ResolveProductOverview.
type ProductOverviewRequest struct {
	TeamID    string
	ProductID string
}

// ResolveProductOverview composes the product page's roll-ups by walking the
// product's implementations once and reusing ResolveImplOverview per impl.
// The per-feature aggregation folds those impl-level summaries by feature name.
//
// Cost: O(impls × features-per-impl) underlying queries — same shape as
// rendering the impl detail page once per impl. Acceptable at typical
// product sizes; batch with a single SQL roll-up when product cardinality
// grows.
func (s *FeatureViewService) ResolveProductOverview(ctx context.Context, req ProductOverviewRequest) (*ProductOverview, error) {
	impls, err := s.impls.ListByProduct(ctx, req.TeamID, req.ProductID)
	if err != nil {
		return nil, fmt.Errorf("services: ResolveProductOverview: list impls: %w", err)
	}

	out := &ProductOverview{}
	featureIdx := map[string]*ProductFeatureSummary{}
	for _, impl := range impls {
		ov, err := s.ResolveImplOverview(ctx, ImplOverviewRequest{Implementation: impl})
		if err != nil {
			return nil, fmt.Errorf("services: ResolveProductOverview: impl %q: %w", impl.Name, err)
		}
		out.Impls = append(out.Impls, &ProductImplSummary{
			Implementation: impl,
			Overview:       ov,
		})
		out.AggregateTotal += ov.AggregateTotal
		out.AggregateCounts.Null += ov.AggregateCounts.Null
		out.AggregateCounts.Assigned += ov.AggregateCounts.Assigned
		out.AggregateCounts.Blocked += ov.AggregateCounts.Blocked
		out.AggregateCounts.Incomplete += ov.AggregateCounts.Incomplete
		out.AggregateCounts.Completed += ov.AggregateCounts.Completed
		out.AggregateCounts.Rejected += ov.AggregateCounts.Rejected
		out.AggregateCounts.Accepted += ov.AggregateCounts.Accepted

		for _, f := range ov.Features {
			fs, ok := featureIdx[f.FeatureName]
			if !ok {
				fs = &ProductFeatureSummary{FeatureName: f.FeatureName}
				featureIdx[f.FeatureName] = fs
				out.Features = append(out.Features, fs)
			}
			fs.ImplCount++
			fs.TotalRequirements += f.TotalRequirements
			fs.Counts.Null += f.Counts.Null
			fs.Counts.Assigned += f.Counts.Assigned
			fs.Counts.Blocked += f.Counts.Blocked
			fs.Counts.Incomplete += f.Counts.Incomplete
			fs.Counts.Completed += f.Counts.Completed
			fs.Counts.Rejected += f.Counts.Rejected
			fs.Counts.Accepted += f.Counts.Accepted
		}
	}
	sort.Slice(out.Impls, func(i, j int) bool {
		return out.Impls[i].Implementation.Name < out.Impls[j].Implementation.Name
	})
	sort.Slice(out.Features, func(i, j int) bool {
		return out.Features[i].FeatureName < out.Features[j].FeatureName
	})
	return out, nil
}

// HeatmapCell is one (product, feature) intersection on the team heatmap.
// Present=false marks cells where the product doesn't have a spec for the
// feature — rendered as an empty slot, not "0% complete".
type HeatmapCell struct {
	ProductName       string
	FeatureName       string
	TotalRequirements int
	Counts            StatusCounts
	Present           bool
}

// HeatmapRow is one product's worth of cells, in the same column order as
// TeamHeatmap.FeatureNames so the view can render a regular grid.
type HeatmapRow struct {
	ProductName    string
	ProductTotal   int
	ProductCounts  StatusCounts
	ProductPresent bool // any cell in this row Present?
	Cells          []*HeatmapCell
}

// TeamHeatmap is the data behind the team-overview heatmap: products as
// rows, the union of all features across products as columns, and a
// top-line aggregate that mirrors the impl/product banners.
type TeamHeatmap struct {
	FeatureNames    []string // sorted, used as columns
	Rows            []*HeatmapRow
	AggregateTotal  int
	AggregateCounts StatusCounts
	ImplCount       int
}

// TeamHeatmapRequest is the input to ResolveTeamHeatmap.
type TeamHeatmapRequest struct {
	TeamID string
}

// ResolveTeamHeatmap composes the team page's products × features grid by
// reusing ResolveProductOverview per product and pivoting its per-feature
// summaries into a column-aligned matrix.
//
// Cost: same as rendering each product page once. For the team-scope this
// is the highest fan-out point in the app; if products × impls × features
// grows, the obvious next move is a single roll-up SQL query keyed by
// (product, feature).
func (s *FeatureViewService) ResolveTeamHeatmap(ctx context.Context, req TeamHeatmapRequest) (*TeamHeatmap, error) {
	prods, err := s.products.ListForTeam(ctx, req.TeamID)
	if err != nil {
		return nil, fmt.Errorf("services: ResolveTeamHeatmap: list products: %w", err)
	}

	type productView struct {
		product  *products.Product
		overview *ProductOverview
	}
	views := make([]productView, 0, len(prods))
	featureSet := map[string]struct{}{}
	implCount := 0
	for _, p := range prods {
		ov, err := s.ResolveProductOverview(ctx, ProductOverviewRequest{
			TeamID:    req.TeamID,
			ProductID: p.ID,
		})
		if err != nil {
			return nil, fmt.Errorf("services: ResolveTeamHeatmap: product %q: %w", p.Name, err)
		}
		views = append(views, productView{product: p, overview: ov})
		implCount += len(ov.Impls)
		for _, f := range ov.Features {
			featureSet[f.FeatureName] = struct{}{}
		}
	}

	featureNames := make([]string, 0, len(featureSet))
	for name := range featureSet {
		featureNames = append(featureNames, name)
	}
	sort.Strings(featureNames)

	out := &TeamHeatmap{
		FeatureNames: featureNames,
		ImplCount:    implCount,
	}

	for _, pv := range views {
		// Build a feature-name → ProductFeatureSummary map for O(1) lookup
		// per column rather than re-walking the slice for each cell.
		byName := make(map[string]*ProductFeatureSummary, len(pv.overview.Features))
		for _, f := range pv.overview.Features {
			byName[f.FeatureName] = f
		}
		row := &HeatmapRow{
			ProductName:    pv.product.Name,
			ProductTotal:   pv.overview.AggregateTotal,
			ProductCounts:  pv.overview.AggregateCounts,
			ProductPresent: len(pv.overview.Features) > 0,
		}
		for _, name := range featureNames {
			cell := &HeatmapCell{ProductName: pv.product.Name, FeatureName: name}
			if f, ok := byName[name]; ok {
				cell.Present = true
				cell.TotalRequirements = f.TotalRequirements
				cell.Counts = f.Counts
			}
			row.Cells = append(row.Cells, cell)
		}
		out.Rows = append(out.Rows, row)

		// Aggregate per cell so we don't double-count empty placeholders.
		out.AggregateTotal += pv.overview.AggregateTotal
		out.AggregateCounts.Null += pv.overview.AggregateCounts.Null
		out.AggregateCounts.Assigned += pv.overview.AggregateCounts.Assigned
		out.AggregateCounts.Blocked += pv.overview.AggregateCounts.Blocked
		out.AggregateCounts.Incomplete += pv.overview.AggregateCounts.Incomplete
		out.AggregateCounts.Completed += pv.overview.AggregateCounts.Completed
		out.AggregateCounts.Rejected += pv.overview.AggregateCounts.Rejected
		out.AggregateCounts.Accepted += pv.overview.AggregateCounts.Accepted
	}

	sort.Slice(out.Rows, func(i, j int) bool { return out.Rows[i].ProductName < out.Rows[j].ProductName })

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
