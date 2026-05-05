package ops_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/ops"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

func TestHealth_ReturnsOKWhenDBIsAlive(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "health_ok.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	h := ops.HealthHandler(db, "test-version")
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_health", http.NoBody)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf(`body["status"] = %q, want "ok"`, body["status"])
	}
	if body["db"] != "ok" {
		t.Errorf(`body["db"] = %q, want "ok"`, body["db"])
	}
	if body["version"] != "test-version" {
		t.Errorf(`body["version"] = %q, want "test-version"`, body["version"])
	}
}

func TestHealth_ReturnsServiceUnavailableWhenDBClosed(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "health_closed.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	// Close the DB before handing it to the handler.
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	h := ops.HealthHandler(db, "v0.0.0")
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_health", http.NoBody)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "degraded" {
		t.Errorf(`body["status"] = %q, want "degraded"`, body["status"])
	}
	if body["db"] != "error" {
		t.Errorf(`body["db"] = %q, want "error"`, body["db"])
	}
}
