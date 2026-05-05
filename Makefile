.PHONY: build test lint vet precommit golden-capture clean

# Build outputs the `acai` binary at repo root.
build:
	go build -trimpath -ldflags="-s -w -X main.version=$$(git describe --always --dirty 2>/dev/null || echo dev)" -o acai ./cmd/acai

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

# `lint` requires golangci-lint to be installed. The CI workflow installs it.
# Locally: `brew install golangci-lint` (macOS) or see https://golangci-lint.run/welcome/install/.
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not installed; see Makefile for install instructions."; \
		exit 1; \
	}
	golangci-lint run

# precommit mirrors the existing `mix precommit` alias on main.
precommit: vet lint test
	@echo "precommit: all checks passed."

# golden-capture refreshes testdata/openapi.golden.json from a running Phoenix
# server on $$ACAI_PHOENIX_BASE_URL (default http://localhost:4000).
# Used during P0 Task 9 and re-used in P2 if the spec changes.
# Uses `jq -S` for sorted-key output if jq is available (stable diffs);
# otherwise falls back to raw curl output.
golden-capture:
	@base="$${ACAI_PHOENIX_BASE_URL:-http://localhost:4000}"; \
	echo "Capturing $$base/api/v1/openapi.json -> testdata/openapi.golden.json"; \
	mkdir -p testdata; \
	if command -v jq >/dev/null 2>&1; then \
	  curl -fsS "$$base/api/v1/openapi.json" | jq -S . > testdata/openapi.golden.json; \
	else \
	  curl -fsS "$$base/api/v1/openapi.json" > testdata/openapi.golden.json; \
	fi
	@echo "Done. Stat:"
	@git diff --stat -- testdata/openapi.golden.json 2>/dev/null || true

clean:
	rm -f acai coverage.txt coverage.html
