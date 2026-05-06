// Package server owns the HTTP listener and chi router lifecycle.
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

	"github.com/alexedwards/scs/v2"

	apimiddleware "github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/api/operations"
	"github.com/jadams-positron/acai-sh-server/internal/config"
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/site/handlers"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

// RouterDeps groups everything newRouter needs.
type RouterDeps struct {
	DB              *store.DB
	Sessions        *scs.SessionManager
	Accounts        *accounts.Repository
	AuthHandlerDeps *handlers.AuthDeps
	CSRFKey         []byte
	SecureCookie    bool
	Version         string
	Teams           *teams.Repository
	Operations      *operations.Config
	APILimiter      apimiddleware.Limiter
}

// Server is the HTTP server bundle.
type Server struct {
	cfg     *config.Config
	logger  *slog.Logger
	db      *store.DB
	version string
	http    *http.Server
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

	router := newRouter(deps)

	httpServer := &http.Server{
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return &Server{
		cfg:     cfg,
		logger:  logger,
		db:      deps.DB,
		version: deps.Version,
		http:    httpServer,
	}, nil
}

// Handler returns the underlying HTTP handler. Useful for httptest.NewServer.
func (s *Server) Handler() http.Handler { return s.http.Handler }

// Run starts the listener on cfg.HTTPPort and blocks until ctx is canceled.
func (s *Server) Run(ctx context.Context, addrCh chan<- string) error {
	addr := net.JoinHostPort("0.0.0.0", strconv.Itoa(s.cfg.HTTPPort))
	lc := &net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("server: listen %s: %w", addr, err)
	}

	if addrCh != nil {
		select {
		case addrCh <- ln.Addr().String():
		default:
		}
	}

	s.logger.Info("server starting", slog.String("addr", ln.Addr().String()), slog.String("version", s.version))

	errCh := make(chan error, 1)
	go func() {
		if err := s.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		s.logger.Info("server stopping")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.http.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server: shutdown: %w", err)
		}
		<-errCh
		return nil
	case err := <-errCh:
		return err
	}
}
