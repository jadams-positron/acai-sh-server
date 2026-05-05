package store

import (
	"context"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/jadams-positron/acai-sh-server/internal/store/migrations"
)

// RunMigrations runs all pending goose migrations against the write pool.
// It is safe to call multiple times; goose tracks applied versions and skips
// migrations that have already run.
func RunMigrations(ctx context.Context, db *DB) error {
	goose.SetBaseFS(migrations.FS)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("store.RunMigrations: set dialect: %w", err)
	}

	if err := goose.UpContext(ctx, db.Write, "."); err != nil {
		return fmt.Errorf("store.RunMigrations: %w", err)
	}

	return nil
}
