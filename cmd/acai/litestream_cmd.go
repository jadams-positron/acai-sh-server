package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	internallitestream "github.com/jadams-positron/acai-sh-server/internal/litestream"
	"github.com/jadams-positron/acai-sh-server/internal/ops"
)

// runLitestreamSubcommand dispatches `acai litestream <sub>` commands.
//
// Supported subcommands:
//
//	acai litestream status     — report replication position
func runLitestreamSubcommand(ctx context.Context, args []string, _, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: acai litestream <status>")
		return 2
	}
	switch args[0] {
	case "status":
		return runLitestreamStatus(ctx, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "litestream: unknown subcommand %q\n", args[0])
		_, _ = fmt.Fprintln(stderr, "usage: acai litestream <status>")
		return 2
	}
}

func runLitestreamStatus(ctx context.Context, stderr io.Writer) int {
	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./acai.db"
	}
	cfg := internallitestream.FromEnv(dbPath)

	logger := ops.SetupLoggerMinimal(stderr)
	if err := internallitestream.Status(ctx, logger, cfg); err != nil {
		_, _ = fmt.Fprintf(stderr, "litestream status: %v\n", err)
		return 1
	}
	return 0
}

// runRestore implements `acai restore --from-s3 --out=<path>`.
func runRestore(ctx context.Context, args []string, _, stderr io.Writer) int {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(stderr)

	fromS3 := fs.Bool("from-s3", false, "restore from the configured S3 bucket (required)")
	outPath := fs.String("out", "", "output file path for the restored database")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !*fromS3 {
		_, _ = fmt.Fprintln(stderr, "restore: --from-s3 is required")
		fs.Usage()
		return 2
	}

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./acai.db"
	}
	cfg := internallitestream.FromEnv(dbPath)
	if cfg == nil {
		_, _ = fmt.Fprintln(stderr, "restore: LITESTREAM_S3_BUCKET is not set")
		return 1
	}

	logger := ops.SetupLoggerMinimal(stderr)
	if err := internallitestream.Restore(ctx, logger, cfg, *outPath); err != nil {
		_, _ = fmt.Fprintf(stderr, "restore: %v\n", err)
		return 1
	}
	return 0
}
