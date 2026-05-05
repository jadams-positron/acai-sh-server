// Package store manages the SQLite database connection pools.
//
// It opens two *sql.DB handles against the same file:
//   - Write: MaxOpenConns(1) — serializes writers, eliminates SQLITE_BUSY
//   - Read:  MaxOpenConns(runtime.NumCPU()*2) — WAL allows many concurrent readers
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"runtime"

	// Register the modernc pure-Go SQLite driver as "sqlite".
	_ "modernc.org/sqlite"
)

// DB holds the read and write connection pools for a single SQLite file.
type DB struct {
	// Read is the multi-connection pool for read-only queries.
	Read *sql.DB
	// Write is the single-connection pool for all mutating queries.
	Write *sql.DB
	path  string
}

// Open opens (or creates) a SQLite database at the given path with WAL mode,
// safe pragmas, and a dual connection-pool setup. It pings both pools before
// returning; any error is reported as a non-nil error value.
func Open(path string) (*DB, error) {
	if path == "" {
		return nil, errors.New("store.Open: path must not be empty")
	}

	writeDSN := buildDSN(path)
	readDSN := buildDSN(path)

	write, err := sql.Open("sqlite", writeDSN)
	if err != nil {
		return nil, fmt.Errorf("store.Open write pool: %w", err)
	}
	write.SetMaxOpenConns(1)

	read, err := sql.Open("sqlite", readDSN)
	if err != nil {
		_ = write.Close()
		return nil, fmt.Errorf("store.Open read pool: %w", err)
	}
	read.SetMaxOpenConns(runtime.NumCPU() * 2)

	ctx := context.Background()
	if err := write.PingContext(ctx); err != nil {
		_ = write.Close()
		_ = read.Close()
		return nil, fmt.Errorf("store.Open write ping: %w", err)
	}
	if err := read.PingContext(ctx); err != nil {
		_ = write.Close()
		_ = read.Close()
		return nil, fmt.Errorf("store.Open read ping: %w", err)
	}

	return &DB{Read: read, Write: write, path: path}, nil
}

// Close closes both the read and write connection pools.
func (db *DB) Close() error {
	werr := db.Write.Close()
	rerr := db.Read.Close()
	if werr != nil {
		return werr
	}
	return rerr
}

// Path returns the filesystem path of the underlying SQLite file.
func (db *DB) Path() string {
	return db.path
}

// buildDSN constructs the SQLite DSN with PRAGMAs encoded as _pragma query params.
func buildDSN(path string) string {
	params := url.Values{}
	params.Set("_pragma", "journal_mode(WAL)")
	params.Add("_pragma", "synchronous(NORMAL)")
	params.Add("_pragma", "foreign_keys(ON)")
	params.Add("_pragma", "busy_timeout(5000)")
	params.Add("_pragma", "temp_store(MEMORY)")
	params.Add("_pragma", "cache_size(-20000)")
	return "file:" + path + "?" + params.Encode()
}
