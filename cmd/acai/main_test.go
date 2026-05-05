package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintVersion_WritesNonEmptyVersion(t *testing.T) {
	var buf bytes.Buffer
	printVersion(&buf, "0.0.0-dev")
	got := buf.String()
	if !strings.Contains(got, "acai") {
		t.Fatalf("printVersion output missing %q: got %q", "acai", got)
	}
	if !strings.Contains(got, "0.0.0-dev") {
		t.Fatalf("printVersion output missing version %q: got %q", "0.0.0-dev", got)
	}
}

func TestRun_VersionFlag_ExitsZeroAndPrints(t *testing.T) {
	var buf bytes.Buffer
	code := run(context.Background(), []string{"acai", "version"}, &buf, io.Discard)
	if code != 0 {
		t.Fatalf("run(version) exit code = %d, want 0", code)
	}
	out := buf.String()
	if !strings.Contains(out, "acai") {
		t.Fatalf("run(version) stdout missing %q: got %q", "acai", out)
	}
	if !strings.Contains(out, "0.0.0-dev") {
		t.Fatalf("run(version) stdout missing version %q: got %q", "0.0.0-dev", out)
	}
}

func TestRun_NoArgs_ShowsUsageAndExitsTwo(t *testing.T) {
	var buf bytes.Buffer
	code := run(context.Background(), []string{"acai"}, &buf, io.Discard)
	if code != 2 {
		t.Fatalf("run() exit code = %d, want 2", code)
	}
	out := buf.String()
	if !strings.Contains(out, "usage:") {
		t.Fatalf("run() output missing %q: got %q", "usage:", out)
	}
	if !strings.Contains(out, "serve") {
		t.Fatalf("run() output missing %q: got %q", "serve", out)
	}
}

func TestRun_UnknownSubcommand_ExitsTwoWithMessage(t *testing.T) {
	var buf bytes.Buffer
	code := run(context.Background(), []string{"acai", "bogus"}, &buf, io.Discard)
	if code != 2 {
		t.Fatalf("run(unknown) exit code = %d, want 2", code)
	}
	out := buf.String()
	if !strings.Contains(out, "unknown subcommand") {
		t.Fatalf("run(unknown) output missing %q: got %q", "unknown subcommand", out)
	}
	if !strings.Contains(out, "usage:") {
		t.Fatalf("run(unknown) output missing %q: got %q", "usage:", out)
	}
}

// TestRun_MigrateSubcommand_RunsMigrationsAndExitsZero exercises the migrate
// subcommand against a temp directory.
func TestRun_MigrateSubcommand_RunsMigrationsAndExitsZero(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	t.Setenv("DATABASE_PATH", dbPath)
	t.Setenv("HTTP_PORT", "4000")
	t.Setenv("LOG_LEVEL", "warn") // quiet logs

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"acai", "migrate"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(migrate) exit code = %d, want 0; stderr=%s", code, stderr.String())
	}

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("DB file not created: %v", err)
	}
}
