package auth_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

func newAccountsRepo(t *testing.T) *accounts.Repository {
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

func TestMagicLinkService_GenerateAndConsume(t *testing.T) {
	repo := newAccountsRepo(t)
	ctx := context.Background()

	user, err := repo.CreateUser(ctx, accounts.CreateUserParams{Email: "dave@example.com"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	svc := auth.NewMagicLinkService(repo, "https://acai.test")

	loginURL, rawToken, err := svc.GenerateLoginURL(ctx, user)
	if err != nil {
		t.Fatalf("GenerateLoginURL: %v", err)
	}
	if rawToken == "" {
		t.Fatal("expected non-empty rawToken")
	}
	wantURLPrefix := "https://acai.test/users/log-in/"
	if !strings.HasPrefix(loginURL, wantURLPrefix) {
		t.Errorf("loginURL = %q, want prefix %q", loginURL, wantURLPrefix)
	}
	if !strings.HasSuffix(loginURL, rawToken) {
		t.Errorf("loginURL = %q, want suffix %q", loginURL, rawToken)
	}

	got, err := svc.ConsumeLoginToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("ConsumeLoginToken: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("ConsumeLoginToken user ID = %q, want %q", got.ID, user.ID)
	}
}
