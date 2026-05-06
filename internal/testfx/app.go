package testfx

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/api/operations"
	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/config"
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/mail"
	"github.com/jadams-positron/acai-sh-server/internal/server"
	"github.com/jadams-positron/acai-sh-server/internal/site/handlers"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

// App is a fully-wired Acai server bundle for tests: DB, repos, echo router.
type App struct {
	t      *testing.T
	DB     *store.DB
	Echo   *echo.Echo
	Server *server.Server // optional — if you want to call srv.Run
	Mailer *captureMailer
	Cfg    *config.Config
	Logger *slog.Logger
}

// captureMailer captures the MagicLinkArgs of every Send call for assertions.
type captureMailer struct{ Sent []mail.MagicLinkArgs }

// SendMagicLink implements mail.Mailer by appending to Sent for test assertions.
func (c *captureMailer) SendMagicLink(_ context.Context, args mail.MagicLinkArgs) error {
	c.Sent = append(c.Sent, args)
	return nil
}

// NewAppOpts overrides defaults for NewApp.
type NewAppOpts struct {
	HTTPPort      int    // default 0 (we don't bind a listener anyway)
	URLScheme     string // default "http"
	NonProdLimits bool
}

// NewApp returns a fully-wired App with all subsystems mounted.
func NewApp(t *testing.T, opts NewAppOpts) *App {
	t.Helper()
	db := NewDB(t)

	if opts.URLScheme == "" {
		opts.URLScheme = "http"
	}

	cfg := &config.Config{
		LogLevel:      "warn",
		HTTPPort:      opts.HTTPPort,
		SecretKeyBase: strings.Repeat("a", 32) + "DEV-ONLY-secret-key-base-for-test",
		URLHost:       "localhost",
		URLScheme:     opts.URLScheme,
		MailNoop:      true,
		MailFromName:  "Acai Test",
		MailFromEmail: "test@acai.test",
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	accountsRepo := accounts.NewRepository(db)
	teamsRepo := teams.NewRepository(db)
	productsRepo := products.NewRepository(db)
	implsRepo := implementations.NewRepository(db)
	sessionStore := auth.NewSessionStore(cfg.SecretKeyBase, false)
	mlSvc := auth.NewMagicLinkService(accountsRepo, "http://localhost")
	mailer := &captureMailer{}
	authDeps := &handlers.AuthDeps{
		Logger:    logger,
		Sessions:  sessionStore,
		Accounts:  accountsRepo,
		MagicLink: mlSvc,
		Mailer:    mailer,
		FromEmail: cfg.MailFromEmail,
		FromName:  cfg.MailFromName,
	}

	srv, err := server.New(cfg, logger, &server.RouterDeps{
		DB:              db,
		Sessions:        sessionStore,
		Accounts:        accountsRepo,
		Teams:           teamsRepo,
		Products:        productsRepo,
		Implementations: implsRepo,
		Operations:      operations.Load(opts.NonProdLimits || cfg.URLScheme == "http"),
		APILimiter:      middleware.NewInProcessLimiter(),
		AuthHandlerDeps: authDeps,
		SecureCookie:    false,
		Version:         "test",
	})
	if err != nil {
		t.Fatalf("testfx.NewApp: server.New: %v", err)
	}

	return &App{
		t:      t,
		DB:     db,
		Echo:   srv.Echo(),
		Server: srv,
		Mailer: mailer,
		Cfg:    cfg,
		Logger: logger,
	}
}

// Client returns an HTTP test client over the App's echo instance.
func (a *App) Client() *Client { return HTTPClient(a.t, a.Echo) }
