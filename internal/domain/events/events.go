// Package events records first-class team activity (push, status changes,
// member additions, etc.) and exposes a per-scope read API for the
// "Recent activity" strips on the team / product / impl pages.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/jadams-positron/acai-sh-server/internal/store"
	"github.com/jadams-positron/acai-sh-server/internal/store/sqlc"
)

// Kind enumerates the recorded event types. Adding a new kind also
// requires a render branch in views.recentActivityRow.
const (
	KindPushSpec       = "push.spec"
	KindPushRefs       = "push.refs"
	KindStatusChanged  = "status.changed"
	KindTokenMinted    = "token.minted"
	KindMemberAdded    = "member.added"
	KindProductCreated = "product.created"
	KindImplCreated    = "impl.created"
)

// Event mirrors a row in events with the joined actor email. Keeping the
// payload as map[string]any (decoded from JSON) lets callers project the
// fields they care about without a per-kind type explosion.
type Event struct {
	ID          string
	TeamID      string
	ProductID   *string
	ImplID      *string
	FeatureName *string
	ActorUserID *string
	ActorEmail  *string
	Kind        string
	Payload     map[string]any
	InsertedAt  time.Time
}

// EmitParams is the input shape for Repository.Emit. Caller fills the
// scope (Team is required; Product/Impl/FeatureName narrow the scope
// for per-page reads), the actor (nil for system events), the kind,
// and a payload that gets JSON-serialized as-is.
type EmitParams struct {
	TeamID      string
	ProductID   *string
	ImplID      *string
	FeatureName *string
	ActorUserID *string
	Kind        string
	Payload     map[string]any
}

// Scope filters a recent-events query. Most-specific non-nil filter wins:
// ImplID > ProductID > TeamID.
type Scope struct {
	TeamID    string
	ProductID *string // nil = team-wide
	ImplID    *string // nil = team-or-product-wide
}

// Repository wraps the generated sqlc methods.
type Repository struct {
	db *store.DB
}

// NewRepository constructs the repo.
func NewRepository(db *store.DB) *Repository { return &Repository{db: db} }

// Emit writes a single event. Errors are returned to the caller; emit
// sites should log-and-continue rather than fail the parent request.
func (r *Repository) Emit(ctx context.Context, p EmitParams) error {
	if p.TeamID == "" {
		return fmt.Errorf("events.Emit: TeamID is required")
	}
	if p.Kind == "" {
		return fmt.Errorf("events.Emit: Kind is required")
	}
	if p.Payload == nil {
		p.Payload = map[string]any{}
	}
	payloadJSON, err := json.Marshal(p.Payload)
	if err != nil {
		return fmt.Errorf("events.Emit: marshal payload: %w", err)
	}

	q := sqlc.New(r.db.Write)
	return q.InsertEvent(ctx, sqlc.InsertEventParams{
		ID:          uuid.New().String(),
		TeamID:      p.TeamID,
		ProductID:   p.ProductID,
		ImplID:      p.ImplID,
		FeatureName: p.FeatureName,
		ActorUserID: p.ActorUserID,
		Kind:        p.Kind,
		Payload:     string(payloadJSON),
		InsertedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// RecentForScope returns the N most-recent events matching the scope.
// Picks the most-specific non-nil filter — impl > product > team.
func (r *Repository) RecentForScope(ctx context.Context, scope Scope, limit int) ([]*Event, error) {
	if limit <= 0 {
		limit = 5
	}
	q := sqlc.New(r.db.Read)
	switch {
	case scope.ImplID != nil:
		rows, err := q.ListEventsForImpl(ctx, sqlc.ListEventsForImplParams{
			ImplID: scope.ImplID,
			Limit:  int64(limit),
		})
		if err != nil {
			return nil, fmt.Errorf("events.RecentForScope: by impl: %w", err)
		}
		out := make([]*Event, 0, len(rows))
		for i := range rows {
			out = append(out, fromImplRow(rows[i]))
		}
		return out, nil
	case scope.ProductID != nil:
		rows, err := q.ListEventsForProduct(ctx, sqlc.ListEventsForProductParams{
			ProductID: scope.ProductID,
			Limit:     int64(limit),
		})
		if err != nil {
			return nil, fmt.Errorf("events.RecentForScope: by product: %w", err)
		}
		out := make([]*Event, 0, len(rows))
		for i := range rows {
			out = append(out, fromProductRow(rows[i]))
		}
		return out, nil
	default:
		rows, err := q.ListEventsForTeam(ctx, sqlc.ListEventsForTeamParams{
			TeamID: scope.TeamID,
			Limit:  int64(limit),
		})
		if err != nil {
			return nil, fmt.Errorf("events.RecentForScope: by team: %w", err)
		}
		out := make([]*Event, 0, len(rows))
		for i := range rows {
			out = append(out, fromTeamRow(rows[i]))
		}
		return out, nil
	}
}

func decodePayload(raw string) map[string]any {
	if raw == "" {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return map[string]any{"_decode_error": err.Error()}
	}
	return m
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func fromTeamRow(r sqlc.ListEventsForTeamRow) *Event {
	return &Event{
		ID:          r.ID,
		TeamID:      r.TeamID,
		ProductID:   r.ProductID,
		ImplID:      r.ImplID,
		FeatureName: r.FeatureName,
		ActorUserID: r.ActorUserID,
		ActorEmail:  r.ActorEmail,
		Kind:        r.Kind,
		Payload:     decodePayload(r.Payload),
		InsertedAt:  parseTime(r.InsertedAt),
	}
}

func fromProductRow(r sqlc.ListEventsForProductRow) *Event {
	return &Event{
		ID:          r.ID,
		TeamID:      r.TeamID,
		ProductID:   r.ProductID,
		ImplID:      r.ImplID,
		FeatureName: r.FeatureName,
		ActorUserID: r.ActorUserID,
		ActorEmail:  r.ActorEmail,
		Kind:        r.Kind,
		Payload:     decodePayload(r.Payload),
		InsertedAt:  parseTime(r.InsertedAt),
	}
}

func fromImplRow(r sqlc.ListEventsForImplRow) *Event {
	return &Event{
		ID:          r.ID,
		TeamID:      r.TeamID,
		ProductID:   r.ProductID,
		ImplID:      r.ImplID,
		FeatureName: r.FeatureName,
		ActorUserID: r.ActorUserID,
		ActorEmail:  r.ActorEmail,
		Kind:        r.Kind,
		Payload:     decodePayload(r.Payload),
		InsertedAt:  parseTime(r.InsertedAt),
	}
}
