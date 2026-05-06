# Acai Server

A self-hostable monolith for tracking ACID specs across implementations and branches. Deploy on a VPS (e.g. Hetzner) or run locally as a devcontainer.

The repo ships two services, orchestrated with `docker-compose`:

- `acai` — Frontend (templ + Datastar) and JSON REST API, single Go binary
- `caddy` — Reverse proxy, terminating TLS and routing to `acai`

SQLite is embedded in the `acai` binary; durability comes from [Litestream](https://litestream.io) replication to S3 (Hetzner Object Storage works). No separate database service.

## Quickstart

> 👉 Want to start shipping ASAP? **Use our [hosted service instead.](https://app.acai.sh)**

Otherwise, choose from one of the deployment options below.

### Devcontainers & DevPods

The easiest way to host a local instance (or multiple in parallel).

**Prerequisites**
- [ ] Docker Desktop or Podman
- [ ] DevPod CLI

**Steps**
1. Create `infra/.env` (copy from `.env.example`) and at minimum set:
   ```sh
   CADDYFILE=devcontainer
   SECRET_KEY_BASE=$(openssl rand -hex 32)
   ```
2. `devpod up .` (from repo root)
3. `ssh server.devpod`
4. `just run` (compiles + serves on `:4000`)
5. Visit `http://localhost:4000`

#### Parallel Devcontainers

Useful for running multiple agents in parallel — each container has its own SQLite DB and git history, so test runs and migrations never clash.

1. Clone the repo separately for each instance:
   ```
   projects/
   ├── server/
   │   └── infra/.env
   ├── server-2/
   │   └── infra/.env
   ```
2. Configure each `.env` to avoid port clashes:
   ```sh
   INSTANCE_NAME=acai-devpod-2
   URL_PORT=4002        # App accessible at localhost:4002 (default 4000)
   HTTP_PORT_EXT=8082   # Avoid Caddy port 80 conflict
   HTTPS_PORT=8443      # Avoid Caddy port 443 conflict
   ```
3. (Optional) For agent git/gh access: `gh auth login` with a PAT, then `gh auth setup-git`.

## Local development without a devcontainer

```sh
# Install code-gen tools (templ, sqlc) and prep generated files.
just install-tools
just generate

# Run the server with reload-on-change.
just run
```

Common recipes (run `just` with no args for the full list):
- `just test` — run the Go test suite
- `just lint` — run golangci-lint v2
- `just precommit` — fmt + fix + vet + lint + test (what CI runs)

## Troubleshooting & Tips

- **Confirm the proxy is working:** `curl http://localhost:4000/_caddy` → `ok: Caddyfile.devcontainer`
- **Confirm the app is healthy:** `curl http://localhost:4000/_health`
- **Local mail:** with `MAIL_NOOP=true` in `infra/.env`, magic-link URLs are logged via slog instead of sent.
- **Litestream off-by-default:** with `LITESTREAM_S3_BUCKET` empty, replication is a no-op (logged once at startup). Set the `LITESTREAM_S3_*` and `AWS_*` variables to enable.
