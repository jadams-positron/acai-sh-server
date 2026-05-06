# Acai Production Cutover Runbook

This runbook documents the one-time procedure for cutting over from the Phoenix
monolith to the Go rewrite. Phoenix ran on Postgres; the Go rewrite uses
embedded SQLite with Litestream replication to S3.

## Prerequisites

- [ ] Postgres database is accessible (live or via a dump loaded into a local container)
- [ ] S3 bucket provisioned for Litestream (Hetzner Object Storage works)
- [ ] `infra/.env` populated from `infra/.env.example` with production values
- [ ] `SECRET_KEY_BASE` generated: `openssl rand -hex 32`
- [ ] DNS for `app.acai.sh` is **not** changed yet — cutover happens after smoke checks pass
- [ ] New Go image builds cleanly: `docker compose -f infra/docker-compose.yml build acai`

## Steps

### 1. Build the new image

```bash
docker compose -f infra/docker-compose.yml build acai
```

Verify the image is approximately 30-40 MB:

```bash
docker images ghcr.io/acai-sh/server
```

### 2. Bring down the Phoenix stack

Keep the Postgres container running for the import step.

```bash
docker compose -f infra/docker-compose.phoenix.yml.bak stop app caddy backup
```

### 3. Run the one-shot Postgres → SQLite import

The `acai import-postgres` subcommand connects to Postgres, streams rows in
FK order, translates types, and optionally verifies the result.

**Note on `users_tokens` / `email_tokens`:** The Phoenix `users_tokens` table
mixed session tokens, login tokens, and email-change tokens in a single table.
The Go rewrite handles sessions via cookies (not stored in the database), so
the import filters out rows where `context = 'session'` and inserts the rest
into the `email_tokens` table. This is intentional and expected.

```bash
docker compose -f infra/docker-compose.yml run --rm acai \
  /acai import-postgres \
    --pg-url="postgres://USER:PASS@db:5432/acai_prod" \
    --out=/data/acai.db \
    --verify
```

Expected output:

```
connected to Postgres
SQLite schema ready
starting import...
  users                          → N rows
  users_tokens                   → M rows   # session rows filtered out
  teams                          → N rows
  ...
verifying row counts...
  users                          OK (N rows)
  ...
verification complete.
```

If the import fails, fix the issue and re-run with `--force` to overwrite the
partial output file.

### 4. Bring down Postgres

```bash
docker compose -f infra/docker-compose.phoenix.yml.bak stop db
```

### 5. Bring up the new stack

```bash
docker compose -f infra/docker-compose.yml up -d acai caddy
```

Watch the logs during startup:

```bash
docker compose -f infra/docker-compose.yml logs -f acai
```

Expected: `{"level":"INFO","msg":"litestream: replication started",...}` and
the HTTP server listening on port 4000.

### 6. Smoke checks

```bash
# Caddy is up and proxying
curl -fsS https://app.acai.sh/_caddy
# Expected: "ok: Caddyfile.prod"

# App health endpoint
curl -fsS https://app.acai.sh/_health | jq
# Expected: {"status":"ok","db":"ok","version":"..."}

# Check recent log lines
docker compose -f infra/docker-compose.yml logs acai | head -50
```

### 7. Verify Litestream replication

```bash
docker compose -f infra/docker-compose.yml exec acai /acai litestream status
```

Expected output reports a recent generation in S3. If Litestream is not yet
configured (LITESTREAM_S3_BUCKET not set), this will print "not configured".

### 8. Update DNS (if needed)

If DNS was being held during cutover, update it now to point to the new server.

### 9. Monitor for 24 hours

Watch error rates in logs:

```bash
docker compose -f infra/docker-compose.yml logs -f acai 2>&1 | grep '"level":"ERROR"'
```

### 10. Post-cutover cleanup

After at least one week with no issues:

```bash
# Remove the Phoenix Postgres volume (IRREVERSIBLE — ensure Litestream is healthy first)
docker volume rm $(docker compose -f infra/docker-compose.phoenix.yml.bak config --volumes | grep postgres)

# Archive the Phoenix compose file
mv infra/docker-compose.phoenix.yml.bak infra/docker-compose.phoenix.yml.archived
```

## Rollback

If the Go rewrite has issues during the cutover window, roll back to Phoenix:

```bash
# Stop the Go stack
docker compose -f infra/docker-compose.yml down

# Rename files so the Phoenix config is active again
mv infra/docker-compose.yml infra/docker-compose.go.yml.bak
mv infra/docker-compose.phoenix.yml.bak infra/docker-compose.yml

# Bring Phoenix back up
docker compose up -d
```

The Postgres data volume (`postgres_data`) is not touched by the Go stack, so
the database is intact. Sessions from the Phoenix era are also intact (session
cookies are still valid if the SECRET_KEY_BASE has not changed).

After rollback:

- Investigate the root cause before attempting the cutover again.
- The SQLite file at `/data/acai.db` in the `acai_data` volume can be inspected
  or discarded; it does not affect Phoenix.

## Reference

### Env vars

| Variable | Description | Required |
|---|---|---|
| `SECRET_KEY_BASE` | HMAC signing key (min 32 bytes) | Yes |
| `DATABASE_PATH` | SQLite file path | No (default `/data/acai.db`) |
| `URL_HOST` | Public hostname | Yes |
| `URL_SCHEME` | `http` or `https` | Yes |
| `MAIL_NOOP` | Set `false` to enable real email | Yes (prod) |
| `MAILGUN_API_KEY` | Mailgun auth key | When `MAIL_NOOP=false` |
| `MAILGUN_DOMAIN` | Mailgun sending domain | When `MAIL_NOOP=false` |
| `LITESTREAM_S3_BUCKET` | S3 bucket for replication | Recommended in prod |
| `LITESTREAM_S3_REGION` | S3 region | When bucket is set |
| `LITESTREAM_S3_ENDPOINT` | Custom S3 endpoint URL | When not AWS |
| `LITESTREAM_S3_PATH` | Object key prefix | When bucket is set |
| `AWS_ACCESS_KEY_ID` | S3 credentials | When bucket is set |
| `AWS_SECRET_ACCESS_KEY` | S3 credentials | When bucket is set |

### Useful commands

```bash
# Check healthcheck exit code
docker compose exec acai /acai healthcheck; echo "exit: $?"

# Restore from S3 to a new file (disaster recovery)
docker compose exec acai /acai restore --from-s3 --out=/data/acai.restored.db

# Run migration against an existing DB
docker compose exec acai /acai migrate

# Run the import with verification (e.g. for a dry-run test)
docker compose run --rm acai /acai import-postgres \
  --pg-url="postgres://..." \
  --out=/tmp/test.db \
  --verify
```
