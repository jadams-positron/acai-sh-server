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

	"github.com/acai-sh/server/internal/config"
	"github.com/acai-sh/server/internal/store"
)

// Server is the HTTP server for the Acai service.
type Server struct {
	cfg     *config.Config
	logger  *slog.Logger
	db      *store.DB
	version string
	http    *http.Server
}

// New constructs a Server, validates its arguments, builds the router, and
// configures the underlying http.Server. It does not start listening.
func New(cfg *config.Config, logger *slog.Logger, db *store.DB, version string) (*Server, error) {
	if cfg == nil {
		return nil, errors.New("server.New: cfg must not be nil")
	}
	if logger == nil {
		return nil, errors.New("server.New: logger must not be nil")
	}
	if db == nil {
		return nil, errors.New("server.New: db must not be nil")
	}

	router := newRouter(db, version)
	httpSrv := &http.Server{
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return &Server{
		cfg:     cfg,
		logger:  logger,
		db:      db,
		version: version,
		http:    httpSrv,
	}, nil
}

// Run opens a TCP listener, optionally sends the bound address to addrCh, then
// serves HTTP until ctx is canceled. It shuts down gracefully with a 10-second
// deadline. Returns nil on graceful shutdown.
func (s *Server) Run(ctx context.Context, addrCh chan<- string) error {
	addr := net.JoinHostPort("0.0.0.0", strconv.Itoa(s.cfg.HTTPPort))
	lc := &net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("server.Run: listen %s: %w", addr, err)
	}

	// Non-blocking send so callers that don't read addrCh aren't blocked.
	if addrCh != nil {
		select {
		case addrCh <- ln.Addr().String():
		default:
		}
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.http.Serve(ln)
	}()

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server.Run: serve: %w", err)
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.http.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server.Run: shutdown: %w", err)
	}
	return nil
}
