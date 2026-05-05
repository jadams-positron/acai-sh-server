// Command acai is the single-binary entrypoint for the Acai server rewrite.
package main

import (
	"fmt"
	"io"
	"os"
)

// version is overridden at build time via -ldflags="-X main.version=...".
var version = "0.0.0-dev"

func main() {
	os.Exit(run(os.Args, os.Stdout))
}

func run(args []string, w io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(w, "usage: acai <subcommand>")
		fmt.Fprintln(w, "subcommands: version")
		return 2
	}
	switch args[1] {
	case "version":
		printVersion(w, version)
		return 0
	default:
		fmt.Fprintf(w, "unknown subcommand: %q\n", args[1])
		fmt.Fprintln(w, "usage: acai <subcommand>")
		return 2
	}
}

func printVersion(w io.Writer, v string) {
	fmt.Fprintf(w, "acai %s\n", v)
}
