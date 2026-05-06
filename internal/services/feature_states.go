package services

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	"github.com/jadams-positron/acai-sh-server/internal/domain/specs"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
)

// validStatuses is the closed enum the API accepts for state.status.
var validStatuses = map[string]struct{}{
	"assigned":   {},
	"blocked":    {},
	"incomplete": {},
	"completed":  {},
	"rejected":   {},
	"accepted":   {},
}

// FeatureStatesService updates feature_impl_states.
type FeatureStatesService struct {
	products *products.Repository
	impls    *implementations.Repository
	specs    *specs.Repository
}

// NewFeatureStatesService constructs the service with required repos.
func NewFeatureStatesService(p *products.Repository, i *implementations.Repository, s *specs.Repository) *FeatureStatesService {
	return &FeatureStatesService{products: p, impls: i, specs: s}
}

// FeatureStatesUpdate captures the inputs for Update.
type FeatureStatesUpdate struct {
	Team               *teams.Team
	ProductName        string
	ImplementationName string
	FeatureName        string
	States             map[string]StateInput
	MaxStates          int
	MaxCommentLength   int
}

// StateInput is one ACID's incoming state.
type StateInput struct {
	Status  *string // nil = clear; non-nil string must be in validStatuses
	Comment *string
}

// FeatureStatesUpdateResult is the resolved view.
type FeatureStatesUpdateResult struct {
	Product        *products.Product
	Implementation *implementations.Implementation
	StatesWritten  int
	Warnings       []string
}

// Sentinel errors callers map to HTTP statuses.
var (
	ErrTooManyStates  = errors.New("services: too many states")
	ErrCommentTooLong = errors.New("services: comment too long")
	ErrInvalidStatus  = errors.New("services: invalid status")
	// ErrProductNotFound, ErrImplementationNotFound — already in feature_context.go.
)

// Update applies the incoming states map to feature_impl_states for the
// specified (product, impl, feature).
func (s *FeatureStatesService) Update(ctx context.Context, req FeatureStatesUpdate) (*FeatureStatesUpdateResult, error) {
	// Semantic caps.
	if req.MaxStates > 0 && len(req.States) > req.MaxStates {
		return nil, fmt.Errorf("%w: got %d, max %d", ErrTooManyStates, len(req.States), req.MaxStates)
	}
	if req.MaxCommentLength > 0 {
		for acid, st := range req.States {
			if st.Comment != nil && len(*st.Comment) > req.MaxCommentLength {
				return nil, fmt.Errorf("%w: comment for %s exceeds %d chars", ErrCommentTooLong, acid, req.MaxCommentLength)
			}
		}
	}
	// Status enum validation.
	for acid, st := range req.States {
		if st.Status != nil {
			if _, ok := validStatuses[*st.Status]; !ok {
				return nil, fmt.Errorf("%w: %q for %s", ErrInvalidStatus, *st.Status, acid)
			}
		}
	}

	// Resolve product + impl.
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

	// Load existing states (if any) so we can merge.
	existing, err := s.specs.GetStates(ctx, impl.ID, req.FeatureName)
	var merged map[string]specs.ACIDState
	switch {
	case err == nil && existing != nil:
		merged = make(map[string]specs.ACIDState, len(existing.States)+len(req.States))
		maps.Copy(merged, existing.States)
	case err != nil && !specs.IsNotFound(err):
		return nil, err
	default:
		merged = make(map[string]specs.ACIDState, len(req.States))
	}

	// Apply incoming.
	for acid, in := range req.States {
		if in.Status == nil && in.Comment == nil {
			// Empty entry → clear this ACID's state entirely.
			delete(merged, acid)
			continue
		}
		merged[acid] = specs.ACIDState{
			Status:  in.Status,
			Comment: in.Comment,
		}
	}

	if err := s.specs.UpsertStates(ctx, impl.ID, req.FeatureName, merged); err != nil {
		return nil, err
	}

	return &FeatureStatesUpdateResult{
		Product:        prod,
		Implementation: impl,
		StatesWritten:  len(req.States),
		Warnings:       []string{},
	}, nil
}
