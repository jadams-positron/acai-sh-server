package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jadams-positron/acai-sh-server/internal/migrate"
)

// runImportPostgres implements the `acai import-postgres` subcommand.
// It performs a one-shot import of a Postgres database into a fresh SQLite file.
//
// Usage:
//
//	acai import-postgres --pg-url=postgres://... --out=/data/acai.db [--force] [--verify]
func runImportPostgres(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("import-postgres", flag.ContinueOnError)
	fs.SetOutput(stderr)

	pgURL := fs.String("pg-url", "", "Postgres connection URL (required)")
	out := fs.String("out", "", "output SQLite file path (required)")
	force := fs.Bool("force", false, "overwrite existing output file")
	verify := fs.Bool("verify", false, "run row-count + join verification after import")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *pgURL == "" || *out == "" {
		_, _ = fmt.Fprintln(stderr, "import-postgres: --pg-url and --out are required")
		fs.Usage()
		return 2
	}

	if !*force {
		if _, err := os.Stat(*out); err == nil {
			_, _ = fmt.Fprintf(stderr,
				"import-postgres: output file already exists at %s; use --force to overwrite\n", *out)
			return 1
		}
	} else {
		_ = os.Remove(*out)
	}

	if err := migrate.PostgresToSQLite(ctx, *pgURL, *out, *verify, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "import-postgres: %v\n", err)
		return 1
	}
	return 0
}
