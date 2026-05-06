package testfx_test

import (
	"context"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/testfx"
)

func TestNewDB_ReturnsOpenDB(t *testing.T) {
	db := testfx.NewDB(t)
	if db == nil {
		t.Fatal("NewDB returned nil")
	}
	if err := db.Read.PingContext(context.Background()); err != nil {
		t.Fatalf("db.Read.Ping: %v", err)
	}
	if err := db.Write.PingContext(context.Background()); err != nil {
		t.Fatalf("db.Write.Ping: %v", err)
	}
}

func TestNewDB_MigrationsApplied(t *testing.T) {
	db := testfx.NewDB(t)
	// If migrations ran, the products table must exist.
	var name string
	row := db.Read.QueryRowContext(context.Background(),
		"SELECT name FROM sqlite_master WHERE type='table' AND name='products'")
	if err := row.Scan(&name); err != nil {
		t.Fatalf("products table not found after migrations: %v", err)
	}
	if name != "products" {
		t.Errorf("table name = %q, want products", name)
	}
}

func TestNewDB_IsolatedAcrossTests(t *testing.T) {
	db1 := testfx.NewDB(t)
	db2 := testfx.NewDB(t)
	if db1.Path() == db2.Path() {
		t.Error("two NewDB calls returned the same path — not isolated")
	}
}
