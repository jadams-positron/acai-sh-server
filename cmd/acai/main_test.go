package main

import (
	"bytes"
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
	code := run([]string{"acai", "version"}, &buf)
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
	code := run([]string{"acai"}, &buf)
	if code != 2 {
		t.Fatalf("run() exit code = %d, want 2", code)
	}
	if !strings.Contains(buf.String(), "usage:") {
		t.Fatalf("run() output missing %q: got %q", "usage:", buf.String())
	}
}

func TestRun_UnknownSubcommand_ExitsTwoWithMessage(t *testing.T) {
	var buf bytes.Buffer
	code := run([]string{"acai", "bogus"}, &buf)
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
