package main

import (
	"context"
	"fmt"
	"io"

	"github.com/acai-sh/server/internal/config"
	"github.com/acai-sh/server/internal/ops"
	"github.com/acai-sh/server/internal/server"
	"github.com/acai-sh/server/internal/store"
)

// runServe boots the HTTP server. Reads config from env, opens DB, applies
// migrations, then starts the chi server. Blocks until ctx is canceled.
func runServe(ctx context.Context, stderr io.Writer) int {
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

	srv, err := server.New(cfg, logger, db, version)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "server.New: %v\n", err)
		return 1
	}

	if err := srv.Run(ctx, nil); err != nil {
		_, _ = fmt.Fprintf(stderr, "server.Run: %v\n", err)
		return 1
	}
	return 0
}
