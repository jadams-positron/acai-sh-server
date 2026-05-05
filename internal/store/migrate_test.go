package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/store"
)

func TestRunMigrations_BringsSchemaUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "migrate.db")

	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Verify a representative table exists.
	var count int
	if err := db.Write.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'`,
	).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("table 'users' not found after migration; count=%d", count)
	}
}

func TestRunMigrations_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "idempotent.db")

	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	if err := store.RunMigrations(ctx, db); err != nil {
		t.Fatalf("RunMigrations (first): %v", err)
	}
	if err := store.RunMigrations(ctx, db); err != nil {
		t.Fatalf("RunMigrations (second, should be idempotent): %v", err)
	}
}
