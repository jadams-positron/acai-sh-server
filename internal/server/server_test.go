package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acai-sh/server/internal/config"
	"github.com/acai-sh/server/internal/server"
	"github.com/acai-sh/server/internal/store"
)

func TestServer_HealthEndpointReturnsOK(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "server_test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cfg := &config.Config{HTTPPort: 0}
	logger := slog.Default()

	srv, err := server.New(cfg, logger, db, "test-v")
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	addrCh := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx, addrCh)
	}()

	addr := <-addrCh
	// Replace [::] with 127.0.0.1 so http.Get resolves on macOS/Linux.
	addr = strings.Replace(addr, "[::]", "127.0.0.1", 1)
	url := fmt.Sprintf("http://%s/_health", addr)

	resp, err := http.Get(url) //nolint:noctx // test helper; context not needed
	if err != nil {
		cancel()
		t.Fatalf("GET /_health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		cancel()
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf(`body["status"] = %q, want "ok"`, body["status"])
	}
	if body["version"] != "test-v" {
		t.Errorf(`body["version"] = %q, want "test-v"`, body["version"])
	}

	cancel()
	if err := <-done; err != nil {
		t.Errorf("srv.Run: %v", err)
	}
}
