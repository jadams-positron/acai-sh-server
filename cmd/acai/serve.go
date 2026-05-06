package main

import (
	"context"
	"fmt"
	"io"

	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/api/operations"
	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/config"
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/mail"
	"github.com/jadams-positron/acai-sh-server/internal/ops"
	"github.com/jadams-positron/acai-sh-server/internal/server"
	"github.com/jadams-positron/acai-sh-server/internal/site/handlers"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

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

	repo := accounts.NewRepository(db)
	teamsRepo := teams.NewRepository(db)
	productsRepo := products.NewRepository(db)
	implsRepo := implementations.NewRepository(db)
	opsCfg := operations.Load(cfg.URLScheme == "http") // non-prod when plain HTTP
	apiLimiter := middleware.NewInProcessLimiter()
	sessionStore := auth.NewSessionStore(cfg.SecretKeyBase, cfg.URLScheme == "https")

	baseURL := cfg.URLScheme + "://" + cfg.URLHost
	if cfg.HTTPPort != 80 && cfg.HTTPPort != 443 && cfg.URLHost == "localhost" {
		baseURL = fmt.Sprintf("%s://%s:%d", cfg.URLScheme, cfg.URLHost, cfg.HTTPPort)
	}
	mlSvc := auth.NewMagicLinkService(repo, baseURL)

	mailer := mail.NewFromConfig(cfg, logger)

	authDeps := &handlers.AuthDeps{
		Logger:    logger,
		Sessions:  sessionStore,
		Accounts:  repo,
		MagicLink: mlSvc,
		Mailer:    mailer,
		FromEmail: cfg.MailFromEmail,
		FromName:  cfg.MailFromName,
	}

	srv, err := server.New(cfg, logger, &server.RouterDeps{
		DB:              db,
		Sessions:        sessionStore,
		Accounts:        repo,
		AuthHandlerDeps: authDeps,
		SecureCookie:    cfg.URLScheme == "https",
		Version:         version,
		Teams:           teamsRepo,
		Products:        productsRepo,
		Implementations: implsRepo,
		Operations:      opsCfg,
		APILimiter:      apiLimiter,
	})
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
