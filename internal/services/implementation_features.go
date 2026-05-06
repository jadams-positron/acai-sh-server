package services

import (
	"context"
	"errors"
	"sort"

	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	"github.com/jadams-positron/acai-sh-server/internal/domain/specs"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
)

// ImplementationFeaturesService lists features for an implementation with
// status counts. Direct lookup (no inheritance walk) in P2b-3.
type ImplementationFeaturesService struct {
	products *products.Repository
	impls    *implementations.Repository
	specs    *specs.Repository
}

// NewImplementationFeaturesService constructs the service with required repos.
func NewImplementationFeaturesService(p *products.Repository, i *implementations.Repository, s *specs.Repository) *ImplementationFeaturesService {
	return &ImplementationFeaturesService{products: p, impls: i, specs: s}
}

// ListFeaturesRequest captures the inputs for List.
type ListFeaturesRequest struct {
	Team               *teams.Team
	ProductName        string
	ImplementationName string
	StatusFilter       []string
	ChangedSinceCommit *string
}

// FeatureSummary is one feature in the implementation's roster.
type FeatureSummary struct {
	FeatureName        string
	Description        *string
	SpecLastSeenCommit *string
	HasLocalSpec       bool
	HasLocalStates     bool
	RefsInherited      bool // always false in P2b-3
	StatesInherited    bool // always false in P2b-3
	RefsCount          int
	TestRefsCount      int
	TotalCount         int
	CompletedCount     int
}

// ListResult is the resolved view ready for the response builder.
type ListResult struct {
	Product        *products.Product
	Implementation *implementations.Implementation
	Features       []*FeatureSummary
}

// List performs the resolution: product → impl → branches → specs+refs+states.
// Returns ErrProductNotFound or ErrImplementationNotFound on 404 cases.
func (s *ImplementationFeaturesService) List(ctx context.Context, req ListFeaturesRequest) (*ListResult, error) {
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

	// Pick the first tracked branch — same heuristic as feature-context.
	// Without inheritance, this is "the impl's data source".
	branch, err := s.specs.FirstTrackedBranch(ctx, impl.ID)
	if err != nil && !errors.Is(err, specs.ErrNotFound) {
		return nil, err
	}

	// Load all specs on the branch (or empty if no branch).
	specsByName := make(map[string]*specs.Spec)
	if branch != nil {
		rows, err := s.specs.ListSpecsForBranch(ctx, branch.ID)
		if err != nil {
			return nil, err
		}
		for _, sp := range rows {
			specsByName[sp.FeatureName] = sp
		}
	}

	// Load all refs on the branch.
	refsByName := make(map[string]*specs.FeatureBranchRef)
	if branch != nil {
		rows, err := s.specs.ListRefsForBranch(ctx, branch.ID)
		if err != nil {
			return nil, err
		}
		for _, rf := range rows {
			refsByName[rf.FeatureName] = rf
		}
	}

	// Load all states on the impl.
	statesRows, err := s.specs.ListStatesForImpl(ctx, impl.ID)
	if err != nil {
		return nil, err
	}
	statesByName := make(map[string]*specs.FeatureImplState, len(statesRows))
	for _, st := range statesRows {
		statesByName[st.FeatureName] = st
	}

	// Union of feature names from specs and states.
	nameSet := map[string]struct{}{}
	for n := range specsByName {
		nameSet[n] = struct{}{}
	}
	for n := range statesByName {
		nameSet[n] = struct{}{}
	}

	out := &ListResult{Product: prod, Implementation: impl}

	for name := range nameSet {
		sp := specsByName[name]
		st := statesByName[name]
		rf := refsByName[name]

		summary := &FeatureSummary{
			FeatureName:    name,
			HasLocalSpec:   sp != nil,
			HasLocalStates: st != nil,
		}

		if sp != nil {
			summary.Description = sp.FeatureDescription
			commit := sp.LastSeenCommit
			summary.SpecLastSeenCommit = &commit
			summary.TotalCount = len(sp.Requirements)

			// changed_since_commit filter: skip if spec last_seen_commit != filter.
			if req.ChangedSinceCommit != nil && commit != *req.ChangedSinceCommit {
				continue
			}
		} else if req.ChangedSinceCommit != nil {
			// No spec, can't match changed_since_commit filter — skip.
			continue
		}

		// Refs counts.
		if rf != nil {
			for _, refList := range rf.Refs {
				for _, r := range refList {
					if r.IsTest {
						summary.TestRefsCount++
					} else {
						summary.RefsCount++
					}
				}
			}
		}

		// States: completed_count = count of ACIDs whose status is completed/accepted.
		if st != nil {
			for _, acidState := range st.States {
				if acidState.Status == nil {
					continue
				}
				switch *acidState.Status {
				case "completed", "accepted":
					summary.CompletedCount++
				}
			}
		}

		// Status filter — keep feature only if at least one ACID's state matches.
		if len(req.StatusFilter) > 0 && !featureMatchesStatusFilter(req.StatusFilter, st, sp) {
			continue
		}

		out.Features = append(out.Features, summary)
	}

	sort.Slice(out.Features, func(i, j int) bool {
		return out.Features[i].FeatureName < out.Features[j].FeatureName
	})

	return out, nil
}

// featureMatchesStatusFilter returns true if at least one ACID in the feature
// has a state status passing filter. If sp has requirements but no state row
// exists, ACIDs are treated as having nil status — so filter "null" matches.
func featureMatchesStatusFilter(filter []string, st *specs.FeatureImplState, sp *specs.Spec) bool {
	// Build the set of ACID statuses to consider.
	acids := map[string]*string{}
	if sp != nil {
		for acid := range sp.Requirements {
			acids[acid] = nil // start as nil
		}
	}
	if st != nil {
		for acid, acidState := range st.States {
			s := acidState.Status
			acids[acid] = s
		}
	}
	for _, status := range acids {
		if MatchesStatusFilter(filter, status) {
			return true
		}
	}
	return false
}
