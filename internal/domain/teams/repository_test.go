package teams_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

func newRepos(t *testing.T) (db *store.DB, ar *accounts.Repository, tr *teams.Repository) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return db, accounts.NewRepository(db), teams.NewRepository(db)
}

func seedUserAndTeam(t *testing.T, ar *accounts.Repository, tr *teams.Repository) (*accounts.User, *teams.Team) {
	t.Helper()
	u, err := ar.CreateUser(context.Background(), accounts.CreateUserParams{
		Email: "owner@example.com", HashedPassword: "",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	team, err := tr.CreateTeam(context.Background(), "team-one")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	return u, team
}

func TestRepository_CreateAndGetTeam(t *testing.T) {
	_, _, tr := newRepos(t)

	team, err := tr.CreateTeam(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if team.Name != "alpha" {
		t.Errorf("Name = %q, want %q", team.Name, "alpha")
	}

	got, err := tr.GetTeamByID(context.Background(), team.ID)
	if err != nil {
		t.Fatalf("GetTeamByID: %v", err)
	}
	if got.ID != team.ID {
		t.Errorf("GetTeamByID id mismatch")
	}
}

func TestRepository_BuildAndVerifyAccessToken(t *testing.T) {
	_, ar, tr := newRepos(t)
	u, team := seedUserAndTeam(t, ar, tr)
	ctx := context.Background()

	plaintext, err := tr.CreateAccessToken(ctx, teams.CreateAccessTokenParams{
		UserID: u.ID,
		TeamID: team.ID,
		Name:   "ci-token",
		Scopes: []string{"push", "read"},
	})
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}
	if plaintext == "" {
		t.Fatal("expected non-empty plaintext token")
	}
	if len(plaintext) < 30 {
		t.Errorf("plaintext token suspiciously short: %d chars", len(plaintext))
	}

	token, gotTeam, err := tr.VerifyAccessToken(ctx, plaintext)
	if err != nil {
		t.Fatalf("VerifyAccessToken (good): %v", err)
	}
	if token.ID == "" || gotTeam.ID != team.ID {
		t.Errorf("VerifyAccessToken returned wrong shapes: token=%+v team=%+v", token, gotTeam)
	}
}

func TestRepository_VerifyAccessToken_RejectsUnknown(t *testing.T) {
	_, _, tr := newRepos(t)
	_, _, err := tr.VerifyAccessToken(context.Background(), "xxxxxxxx.notarealsecretvaluetomatchanything")
	if err == nil {
		t.Fatal("VerifyAccessToken with unknown token should error")
	}
	if !teams.IsInvalidToken(err) {
		t.Errorf("expected teams.IsInvalidToken, got: %v", err)
	}
}

func TestRepository_VerifyAccessToken_RejectsRevoked(t *testing.T) {
	_, ar, tr := newRepos(t)
	u, team := seedUserAndTeam(t, ar, tr)
	ctx := context.Background()

	plaintext, err := tr.CreateAccessToken(ctx, teams.CreateAccessTokenParams{
		UserID: u.ID, TeamID: team.ID, Name: "to-revoke",
	})
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}

	// prefix is the first 8 url-safe-b64 chars before the dot
	dot := -1
	for i, c := range plaintext {
		if c == '.' {
			dot = i
			break
		}
	}
	if dot <= 0 {
		t.Fatalf("plaintext token has no '.' separator: %q", plaintext)
	}

	if err := tr.RevokeAccessTokenByPrefix(ctx, plaintext[:dot]); err != nil {
		t.Fatalf("RevokeAccessTokenByPrefix: %v", err)
	}

	_, _, err = tr.VerifyAccessToken(ctx, plaintext)
	if err == nil {
		t.Fatal("VerifyAccessToken on revoked should error")
	}
}

func TestRepository_VerifyAccessToken_RejectsExpired(t *testing.T) {
	_, ar, tr := newRepos(t)
	u, team := seedUserAndTeam(t, ar, tr)
	ctx := context.Background()

	past := time.Now().UTC().Add(-1 * time.Hour)
	plaintext, err := tr.CreateAccessToken(ctx, teams.CreateAccessTokenParams{
		UserID: u.ID, TeamID: team.ID, Name: "expired",
		ExpiresAt: &past,
	})
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}

	_, _, err = tr.VerifyAccessToken(ctx, plaintext)
	if err == nil {
		t.Fatal("VerifyAccessToken on expired should error")
	}
}
