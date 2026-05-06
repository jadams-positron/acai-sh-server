// Package server owns the HTTP listener and echo lifecycle.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"

	"github.com/jadams-positron/acai-sh-server/internal/api"
	apimiddleware "github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/api/operations"
	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/config"
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/ops"
	"github.com/jadams-positron/acai-sh-server/internal/site"
	"github.com/jadams-positron/acai-sh-server/internal/site/handlers"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

// RouterDeps groups everything New needs.
type RouterDeps struct {
	DB              *store.DB
	Sessions        *auth.SessionStore
	Accounts        *accounts.Repository
	AuthHandlerDeps *handlers.AuthDeps
	SecureCookie    bool
	Version         string
	Teams           *teams.Repository
	Products        *products.Repository
	Implementations *implementations.Repository
	Operations      *operations.Config
	APILimiter      apimiddleware.Limiter
}

// Server is the HTTP server bundle.
type Server struct {
	cfg     *config.Config
	logger  *slog.Logger
	db      *store.DB
	version string
	echo    *echo.Echo
}

// New constructs a *Server with all dependencies wired.
func New(cfg *config.Config, logger *slog.Logger, deps *RouterDeps) (*Server, error) {
	if cfg == nil {
		return nil, errors.New("server: cfg is nil")
	}
	if logger == nil {
		return nil, errors.New("server: logger is nil")
	}
	if deps == nil || deps.DB == nil {
		return nil, errors.New("server: deps with DB are required")
	}

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(echomiddleware.RequestID())
	e.Use(echomiddleware.Recover())

	// CSRF middleware — used selectively on browser routes via a Group.
	// TokenLookup uses the gorilla-compatible form field name for wire compatibility
	// with existing templates. The string is a config key, not a credential.
	csrfMW := echomiddleware.CSRFWithConfig(echomiddleware.CSRFConfig{ //nolint:gosec // G101 false positive: TokenLookup is a config key, not a credential
		TokenLookup:    "form:gorilla.csrf.Token",
		CookieName:     "_acai_csrf",
		CookieHTTPOnly: true,
		CookieSecure:   deps.SecureCookie,
		CookieSameSite: http.SameSiteLaxMode,
	})

	// Browser group: load scope, then individual route groups apply csrf as needed.
	browser := e.Group("", auth.LoadScope(deps.Sessions, deps.Accounts))
	site.MountAuthRoutes(browser, deps.AuthHandlerDeps, csrfMW)
	site.MountAuthRequiredStub(browser, csrfMW)

	// Static assets — public, no session.
	handlers.MountStatic(e.Group(""))

	// Health check — outside session middleware.
	e.GET("/_health", ops.HealthHandlerEcho(deps.DB, deps.Version))

	// API tree — bearer auth applied inside Mount.
	api.Mount(e, &api.Deps{
		Teams:           deps.Teams,
		Products:        deps.Products,
		Implementations: deps.Implementations,
		Operations:      deps.Operations,
		Limiter:         deps.APILimiter,
	})

	return &Server{cfg: cfg, logger: logger, db: deps.DB, version: deps.Version, echo: e}, nil
}

// Handler returns the underlying HTTP handler. Useful for httptest.NewServer.
func (s *Server) Handler() http.Handler { return s.echo }

// Run starts the listener on cfg.HTTPPort and blocks until ctx is canceled.
func (s *Server) Run(ctx context.Context, addrCh chan<- string) error {
	addr := net.JoinHostPort("0.0.0.0", strconv.Itoa(s.cfg.HTTPPort))
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("server: listen: %w", err)
	}
	if addrCh != nil {
		select {
		case addrCh <- ln.Addr().String():
		default:
		}
	}
	s.logger.Info("server starting", slog.String("addr", ln.Addr().String()), slog.String("version", s.version))
	s.echo.Listener = ln
	errCh := make(chan error, 1)
	go func() {
		if err := s.echo.Start(""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case <-ctx.Done():
		s.logger.Info("server stopping")
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.echo.Shutdown(shutCtx)
		<-errCh
		return nil
	case err := <-errCh:
		return err
	}
}
