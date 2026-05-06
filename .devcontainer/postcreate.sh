#!/usr/bin/env bash
set -euo pipefail

# Make sure ~/.local/bin is on PATH for this session
export PATH="$HOME/.local/bin:$PATH"

# Install code-generation tools pinned to versions used in CI / Dockerfile.
go install github.com/a-h/templ/cmd/templ@v0.3.1001
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1

# Pre-warm modules + generated code so `just run` works on first invocation.
go mod download
templ generate
sqlc generate

curl -fsSL https://opencode.ai/install | bash
