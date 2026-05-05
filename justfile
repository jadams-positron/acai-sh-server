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

# Install code-generation tools:
#   - templ@v0.3.1001    (Go HTML template compiler)
#   - sqlc@v1.31.1       (SQL-to-Go type-safe query generator)
#   - tailwindcss        (standalone CLI, downloaded to $GOPATH/bin)
# Run once after a fresh clone or when pinned versions change.
install-tools:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "install-tools: installing templ@v0.3.1001 ..."
    go install github.com/a-h/templ/cmd/templ@v0.3.1001
    echo "install-tools: installing sqlc@v1.31.1 ..."
    go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
    echo "install-tools: downloading tailwindcss standalone CLI ..."
    arch="$(uname -m)"
    case "$arch" in
        arm64)  tw_arch="macos-arm64" ;;
        x86_64) tw_arch="macos-x86_64" ;;
        *)      echo "Unsupported arch: $arch"; exit 1 ;;
    esac
    tw_bin="$(go env GOPATH)/bin/tailwindcss"
    curl -fsSL "https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-${tw_arch}" \
        -o "$tw_bin"
    chmod +x "$tw_bin"
    echo "install-tools: done."
    templ --version
    sqlc version
    tailwindcss --help | head -1

# Generate templ → Go bindings. Run after editing *.templ files.
gen-templ:
    templ generate ./internal/site/views/...
    @echo "gen-templ: done."

# Compile Tailwind CSS. Run after adding/removing classes in views or handlers.
gen-css:
    tailwindcss -i assets/css/app.css -o assets/dist/app.css --minify
    @echo "gen-css: done."

# Run code generators: templ, sqlc, tailwindcss.
# Run whenever queries/, migrations/, *.templ, or CSS @source paths change.
gen:
    templ generate ./internal/site/views/... || true
    sqlc generate
    tailwindcss -i assets/css/app.css -o assets/dist/app.css --minify
    @echo "gen: all generators done."

# Remove built artifacts.
clean:
    rm -f acai coverage.txt coverage.html
