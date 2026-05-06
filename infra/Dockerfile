# syntax=docker/dockerfile:1.7

# --- build stage ---
FROM golang:1.26-bookworm AS build
WORKDIR /src

# Cache module downloads as a separate layer.
COPY go.mod go.sum ./
RUN go mod download

# Install code-generation tools pinned to the same versions as justfile.
RUN go install github.com/a-h/templ/cmd/templ@v0.3.1001 \
 && go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1

# Copy full source tree.
COPY . .

# Generate templ + sqlc then build the static binary.
# git describe is best-effort; falls back to "dev" when .git is absent (most CI).
RUN templ generate \
 && sqlc generate \
 && CGO_ENABLED=0 go build \
      -trimpath \
      -ldflags="-s -w -X main.version=$(git -C /src describe --tags --always --dirty 2>/dev/null || echo dev)" \
      -o /out/acai \
      ./cmd/acai

# Pre-create /data owned by the runtime nonroot user (uid 65532). When this
# directory is copied to the runtime stage and a Docker named volume is mounted
# over it, the volume inherits the directory's ownership — so SQLite (running
# as nonroot) can write the DB file. Without this step, Docker creates /data
# as root-owned and the server fails with SQLITE_CANTOPEN.
RUN mkdir -p /out/data \
 && chown -R 65532:65532 /out/data

# --- runtime stage ---
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
USER nonroot:nonroot

COPY --from=build /out/acai /acai
COPY --from=build --chown=65532:65532 /out/data /data

ENV DATABASE_PATH=/data/acai.db \
    HTTP_PORT=4000

EXPOSE 4000

ENTRYPOINT ["/acai"]
CMD ["serve"]
