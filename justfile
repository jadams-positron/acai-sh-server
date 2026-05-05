# justfile — task runner for the Acai server (Go rewrite).
# Run `just` (no args) to see available recipes.

# Default recipe: list all available recipes when `just` is invoked with no arguments.
default:
    @just --list

# Build outputs the `acai` binary at repo root.
build:
    go build -trimpath -ldflags="-s -w -X main.version=$(git describe --always --dirty 2>/dev/null || echo dev)" -o acai ./cmd/acai

# Run gofmt-style auto-formatting (uses `gofmt -s -w` for simplification + canonicalization).
fmt:
    gofmt -s -w .

# Run `go fix` — applies migrations defined in golang.org/x/tools/cmd/fix.
# Mandated to run before every commit (enforced via .githooks/pre-commit).
fix:
    go fix ./...

# Run `go vet` static checks.
vet:
    go vet ./...

# Run golangci-lint. Requires golangci-lint v2.x to be installed.
# Locally: `brew install golangci-lint` (macOS) or see https://golangci-lint.run/welcome/install/.
lint:
    @command -v golangci-lint >/dev/null 2>&1 || { \
        echo "golangci-lint not installed; see https://golangci-lint.run/welcome/install/"; \
        exit 1; \
    }
    golangci-lint run

# Run all tests with race detector and no caching.
test:
    go test -race -count=1 ./...

# precommit mirrors the existing `mix precommit` discipline plus go fix.
# Order: fmt (canonicalize) -> fix (apply migrations) -> vet -> lint -> test.
precommit: fmt fix vet lint test
    @echo "precommit: all checks passed."

# golden-capture refreshes testdata/openapi.golden.json from a running Phoenix
# server on $ACAI_PHOENIX_BASE_URL (default https://app.acai.sh).
# Uses `jq -S` for sorted-key output if jq is available (stable diffs);
# otherwise falls back to raw curl output.
golden-capture:
    #!/usr/bin/env bash
    set -euo pipefail
    base="${ACAI_PHOENIX_BASE_URL:-https://app.acai.sh}"
    echo "Capturing $base/api/v1/openapi.json -> testdata/openapi.golden.json"
    mkdir -p testdata
    if command -v jq >/dev/null 2>&1; then
      curl -fsS "$base/api/v1/openapi.json" | jq -S . > testdata/openapi.golden.json
    else
      curl -fsS "$base/api/v1/openapi.json" > testdata/openapi.golden.json
    fi
    echo "Done."

# Install the project git pre-commit hook by pointing core.hooksPath to .githooks/.
# Run once per fresh clone.
init-hooks:
    git config core.hooksPath .githooks
    @echo "core.hooksPath set to .githooks. Hooks will run on git commit."

# Run code generators (sqlc). Run this whenever queries/ or migrations/ change.
gen:
    sqlc generate
    @echo "gen: sqlc done."

# Remove built artifacts.
clean:
    rm -f acai coverage.txt coverage.html
