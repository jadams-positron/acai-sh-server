package events_test

import (
	"context"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/domain/events"
	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

func TestRepository_EmitAndRecent(t *testing.T) {
	db := testfx.NewDB(t)
	repo := events.NewRepository(db)
	ctx := context.Background()

	user := testfx.SeedUser(t, db, testfx.SeedUserOpts{Email: "events@test.example"})
	team := testfx.SeedTeam(t, db, testfx.SeedTeamOpts{Name: "events-team"})
	product := testfx.SeedProduct(t, db, team, testfx.SeedProductOpts{Name: "events-prod"})
	impl := testfx.SeedImplementation(t, db, product, testfx.SeedImplementationOpts{Name: "events-impl"})

	// Emit one team-scoped, one product-scoped, one impl-scoped.
	if err := repo.Emit(ctx, events.EmitParams{
		TeamID:      team.ID,
		ActorUserID: &user.ID,
		Kind:        events.KindTokenMinted,
		Payload:     map[string]any{"token_name": "ci"},
	}); err != nil {
		t.Fatalf("emit team event: %v", err)
	}
	productID := product.ID
	if err := repo.Emit(ctx, events.EmitParams{
		TeamID:      team.ID,
		ProductID:   &productID,
		ActorUserID: &user.ID,
		Kind:        events.KindProductCreated,
		Payload:     map[string]any{"product_name": product.Name},
	}); err != nil {
		t.Fatalf("emit product event: %v", err)
	}
	implID := impl.ID
	if err := repo.Emit(ctx, events.EmitParams{
		TeamID:      team.ID,
		ProductID:   &productID,
		ImplID:      &implID,
		ActorUserID: &user.ID,
		Kind:        events.KindStatusChanged,
		Payload:     map[string]any{"acid": "AC-1", "to": "completed"},
	}); err != nil {
		t.Fatalf("emit impl event: %v", err)
	}

	// Team-wide read sees all 3.
	teamEvents, err := repo.RecentForScope(ctx, events.Scope{TeamID: team.ID}, 10)
	if err != nil {
		t.Fatalf("RecentForScope team: %v", err)
	}
	if len(teamEvents) != 3 {
		t.Errorf("team scope: want 3 events, got %d", len(teamEvents))
	}
	// Newest first.
	if len(teamEvents) > 0 && teamEvents[0].Kind != events.KindStatusChanged {
		t.Errorf("team scope: want newest=status.changed, got %q", teamEvents[0].Kind)
	}

	// Product scope filters to product+impl events.
	prodEvents, err := repo.RecentForScope(ctx, events.Scope{TeamID: team.ID, ProductID: &productID}, 10)
	if err != nil {
		t.Fatalf("RecentForScope product: %v", err)
	}
	if len(prodEvents) != 2 {
		t.Errorf("product scope: want 2 events, got %d", len(prodEvents))
	}

	// Impl scope filters to one event.
	implEvents, err := repo.RecentForScope(ctx, events.Scope{TeamID: team.ID, ImplID: &implID}, 10)
	if err != nil {
		t.Fatalf("RecentForScope impl: %v", err)
	}
	if len(implEvents) != 1 {
		t.Errorf("impl scope: want 1 event, got %d", len(implEvents))
	}
	if len(implEvents) > 0 {
		ev := implEvents[0]
		if ev.ActorEmail == nil || *ev.ActorEmail != "events@test.example" {
			t.Errorf("expected actor email joined; got %v", ev.ActorEmail)
		}
		if ev.Payload["acid"] != "AC-1" {
			t.Errorf("expected acid=AC-1 in payload; got %v", ev.Payload)
		}
	}
}
