package main

import (
	"context"
	"fmt"
	"io"

	"github.com/acai-sh/server/internal/config"
	"github.com/acai-sh/server/internal/ops"
	"github.com/acai-sh/server/internal/store"
)

// runMigrate opens the DB and runs all pending goose migrations, then exits.
// Idempotent — running against an already-migrated DB is a no-op.
func runMigrate(ctx context.Context, stderr io.Writer) int {
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "config: %v\n", err)
		return 1
	}

	logger := ops.SetupLogger(cfg, stderr)

	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "store.Open: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	if err := store.RunMigrations(ctx, db); err != nil {
		_, _ = fmt.Fprintf(stderr, "store.RunMigrations: %v\n", err)
		return 1
	}

	logger.Info("migrate complete", "database_path", cfg.DatabasePath)
	return 0
}
