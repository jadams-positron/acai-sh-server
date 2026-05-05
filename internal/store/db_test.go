package store_test

import (
	"path/filepath"
	"testing"

	"github.com/acai-sh/server/internal/store"
)

func TestOpen_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error: %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if db.Path() != path {
		t.Errorf("db.Path() = %q, want %q", db.Path(), path)
	}
}

func TestOpen_AppliesPragmas(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pragmas.db")

	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error: %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var journalMode string
	if err := db.Write.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}

	var foreignKeys int
	if err := db.Write.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}
}

func TestOpen_WritePoolHasOneConn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pool.db")

	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error: %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })

	stats := db.Write.Stats()
	if stats.MaxOpenConnections != 1 {
		t.Errorf("Write pool MaxOpenConnections = %d, want 1", stats.MaxOpenConnections)
	}
}

func TestOpen_RejectsEmptyPath(t *testing.T) {
	_, err := store.Open("")
	if err == nil {
		t.Fatal("Open(\"\") should have returned error, got nil")
	}
}
