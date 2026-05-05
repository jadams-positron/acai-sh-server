package accounts_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

func newRepo(t *testing.T) *accounts.Repository {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return accounts.NewRepository(db)
}

func TestRepository_CreateAndGetUser(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	params := accounts.CreateUserParams{
		Email: "alice@example.com",
	}
	u, err := repo.CreateUser(ctx, params)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == "" {
		t.Error("expected non-empty ID")
	}
	if u.Email != "alice@example.com" {
		t.Errorf("Email = %q, want %q", u.Email, "alice@example.com")
	}

	got, err := repo.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("GetUserByID ID = %q, want %q", got.ID, u.ID)
	}
}

func TestRepository_GetUserByEmail_NotFound(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	_, err := repo.GetUserByEmail(ctx, "nobody@example.com")
	if !accounts.IsNotFound(err) {
		t.Errorf("expected IsNotFound error, got %v", err)
	}
}

func TestRepository_BuildAndConsumeMagicLinkToken(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	u, err := repo.CreateUser(ctx, accounts.CreateUserParams{Email: "bob@example.com"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	rawToken, err := repo.BuildEmailToken(ctx, u, "login")
	if err != nil {
		t.Fatalf("BuildEmailToken: %v", err)
	}
	if rawToken == "" {
		t.Fatal("expected non-empty rawToken")
	}

	got, err := repo.ConsumeEmailToken(ctx, rawToken, "login", 15*time.Minute)
	if err != nil {
		t.Fatalf("ConsumeEmailToken: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("ConsumeEmailToken user ID = %q, want %q", got.ID, u.ID)
	}
}

func TestRepository_ConsumeEmailToken_RejectsExpired(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	u, err := repo.CreateUser(ctx, accounts.CreateUserParams{Email: "carol@example.com"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	rawToken, err := repo.BuildEmailToken(ctx, u, "login")
	if err != nil {
		t.Fatalf("BuildEmailToken: %v", err)
	}

	_, err = repo.ConsumeEmailToken(ctx, rawToken, "login", 0)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}
