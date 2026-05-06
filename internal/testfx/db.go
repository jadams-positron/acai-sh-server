// Package testfx is the shared test harness: in-memory SQLite, fixture
// seeders, HTTP test client, and golden-file helpers. Helpers always take
// *testing.T and call t.Fatalf on errors so test bodies stay flat.
package testfx

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/store"
)

// NewDB returns a fresh *store.DB backed by a temp-file SQLite. Migrations
// are applied. t.Cleanup closes the DB.
func NewDB(t *testing.T) *store.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("testfx: store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := store.RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("testfx: RunMigrations: %v", err)
	}
	return db
}
