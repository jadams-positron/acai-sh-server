// Command acai is the single-binary entrypoint for the Acai server.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// version is overridden at build time via -ldflags="-X main.version=...".
var version = "0.0.0-dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		_, _ = fmt.Fprintln(stdout, "usage: acai <subcommand>")
		_, _ = fmt.Fprintln(stdout, "subcommands: serve, migrate, create-admin, version")
		return 2
	}
	switch args[1] {
	case "version":
		printVersion(stdout, version)
		return 0
	case "serve":
		return runServe(ctx, stderr)
	case "migrate":
		return runMigrate(ctx, stderr)
	case "create-admin":
		return runCreateAdmin(ctx, args[2:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stdout, "unknown subcommand: %q\n", args[1])
		_, _ = fmt.Fprintln(stdout, "usage: acai <subcommand>")
		return 2
	}
}

func printVersion(w io.Writer, v string) {
	_, _ = fmt.Fprintf(w, "acai %s\n", v)
}
