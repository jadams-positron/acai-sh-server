package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"io"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/config"
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/ops"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

// runCreateAdmin parses --email + optional --password, opens the DB, runs
// migrations, hashes the password (random 32-byte token if not given), and
// inserts the user.
func runCreateAdmin(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	fs.SetOutput(stderr)
	email := fs.String("email", "", "email address (required)")
	password := fs.String("password", "", "password (random if blank)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *email == "" {
		_, _ = fmt.Fprintln(stderr, "create-admin: --email is required")
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "config: %v\n", err)
		return 1
	}
	logger := ops.SetupLogger(cfg, stderr)
	_ = logger

	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "store.Open: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	if err := store.RunMigrations(ctx, db); err != nil {
		_, _ = fmt.Fprintf(stderr, "store.RunMigrations: %v\n", err)
		return 1
	}

	rawPassword := *password
	if rawPassword == "" {
		buf := make([]byte, 24)
		if _, err := rand.Read(buf); err != nil {
			_, _ = fmt.Fprintf(stderr, "create-admin: random password: %v\n", err)
			return 1
		}
		rawPassword = base64.RawURLEncoding.EncodeToString(buf)
	}

	hash, err := auth.HashPassword(rawPassword)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "create-admin: hash: %v\n", err)
		return 1
	}

	repo := accounts.NewRepository(db)
	user, err := repo.CreateUser(ctx, accounts.CreateUserParams{
		Email:          *email,
		HashedPassword: hash,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "create-admin: insert: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintf(stdout, "user created: %s (%s)\n", user.Email, user.ID)
	if *password == "" {
		_, _ = fmt.Fprintf(stdout, "generated password: %s\n", rawPassword)
		_, _ = fmt.Fprintln(stdout, "(use the magic-link login flow; this password is only for completeness)")
	}
	return 0
}
