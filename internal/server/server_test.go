package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/api/operations"
	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/config"
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/mail"
	"github.com/jadams-positron/acai-sh-server/internal/ops"
	"github.com/jadams-positron/acai-sh-server/internal/server"
	"github.com/jadams-positron/acai-sh-server/internal/site/handlers"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

func newTestServer(t *testing.T) (*server.Server, *store.DB) {
	t.Helper()

	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	cfg := &config.Config{
		HTTPPort:      0,
		LogLevel:      "warn",
		SecretKeyBase: strings.Repeat("a", 32) + "-test-secret-key-base-do-not-use",
		URLHost:       "localhost",
		URLScheme:     "http",
		MailNoop:      true,
	}
	logger := ops.SetupLogger(cfg, io.Discard)
	repo := accounts.NewRepository(db)
	sessionManager := auth.NewSessionManager(db, false)
	mlSvc := auth.NewMagicLinkService(repo, "http://localhost")
	authDeps := &handlers.AuthDeps{
		Logger:    logger,
		Sessions:  sessionManager,
		Accounts:  repo,
		MagicLink: mlSvc,
		Mailer:    mail.NewNoop(logger),
	}

	srv, err := server.New(cfg, logger, &server.RouterDeps{
		DB:              db,
		Sessions:        sessionManager,
		Accounts:        repo,
		AuthHandlerDeps: authDeps,
		CSRFKey:         []byte(cfg.SecretKeyBase[:32]),
		SecureCookie:    false,
		Version:         "test-version",
		Teams:           teams.NewRepository(db),
		Operations:      operations.Load(true),
		APILimiter:      middleware.NewInProcessLimiter(),
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return srv, db
}

func TestServer_HealthEndpointReturnsOK(t *testing.T) {
	srv, _ := newTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	addrCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run(ctx, addrCh)
	}()

	addr := <-addrCh

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/_health", http.NoBody)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /_health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, body)
	}

	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["status"] != "ok" || got["db"] != "ok" || got["version"] != "test-version" {
		t.Errorf("unexpected body: %+v", got)
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Run returned non-shutdown error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Errorf("server did not shut down within 3s")
	}
}
