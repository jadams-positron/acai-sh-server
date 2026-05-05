package main

import (
	"context"
	"fmt"
	"io"

	"github.com/acai-sh/server/internal/auth"
	"github.com/acai-sh/server/internal/config"
	"github.com/acai-sh/server/internal/domain/accounts"
	"github.com/acai-sh/server/internal/mail"
	"github.com/acai-sh/server/internal/ops"
	"github.com/acai-sh/server/internal/server"
	"github.com/acai-sh/server/internal/site/handlers"
	"github.com/acai-sh/server/internal/store"
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
	sessionManager := auth.NewSessionManager(db, cfg.URLScheme == "https")

	baseURL := cfg.URLScheme + "://" + cfg.URLHost
	if cfg.HTTPPort != 80 && cfg.HTTPPort != 443 && cfg.URLHost == "localhost" {
		baseURL = fmt.Sprintf("%s://%s:%d", cfg.URLScheme, cfg.URLHost, cfg.HTTPPort)
	}
	mlSvc := auth.NewMagicLinkService(repo, baseURL)

	var mailer mail.Mailer = mail.NewNoop(logger)
	if !cfg.MailNoop {
		_, _ = fmt.Fprintln(stderr, "warning: MAIL_NOOP=false but no production mailer is wired in P1b; using noop")
	}

	authDeps := &handlers.AuthDeps{
		Logger:    logger,
		Sessions:  sessionManager,
		Accounts:  repo,
		MagicLink: mlSvc,
		Mailer:    mailer,
		FromEmail: cfg.MailFromEmail,
		FromName:  cfg.MailFromName,
	}

	srv, err := server.New(cfg, logger, &server.RouterDeps{
		DB:              db,
		Sessions:        sessionManager,
		Accounts:        repo,
		AuthHandlerDeps: authDeps,
		CSRFKey:         []byte(cfg.SecretKeyBase[:32]),
		SecureCookie:    cfg.URLScheme == "https",
		Version:         version,
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
