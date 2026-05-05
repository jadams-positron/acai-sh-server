# Go + Datastar Rewrite — Design Spec

**Status:** Approved (sections 1–9, all confirmed by user)
**Author:** Jason Adams
**Date:** 2026-05-05
**Scope:** Replace the Acai Phoenix monolith with a single Go binary that serves the public REST API and a Datastar-driven site, backed by SQLite.

---

## 1. Context & goals

### Drivers
- **Page loads feel slow** on the current Phoenix/LiveView stack (unmeasured; suspected: LiveView initial-HTTP render + JS+websocket bootstrap, plus devcontainer FS overhead on macOS).
- **User preference**: Go performance, Datastar's SSE-first reactive model.
- **Operational simplicity**: collapse the 5-service docker-compose into a 2-service stack with a single SQLite file.

### Constraints (locked with user)
- **API contract preserved exactly.** A CLI tool consumes `/api/v1/*` and must keep working with no edits. Endpoints, request/response shapes, status codes, headers, error formats, and OpenAPI spec all carry over.
- **Data preserved.** A one-shot Postgres → SQLite import migrates the user's existing project. Data shape may change; the import is responsible for the translation.
- **Single user / pre-production scale.** "Single project so far" — clean break is acceptable, no parallel-stack period.

### Approach selected

**Approach 1: Big-bang single-binary rewrite** (selected over API-first phasing or strangler-fig). Rationale: single user, finite scope (5 API endpoints, 12 LiveViews, ~7 domain contexts), an API contract suite is the safety net, no production blast radius. Implementation is sequenced internally (DB → API → site → cutover) but only one cutover happens.

### Success criteria
- All 5 `/api/v1/*` endpoints respond with byte-identical JSON shapes to the current Phoenix stack on a captured set of fixtures.
- `TestOpenAPISpecParity` passes.
- The user's CLI tool runs successfully against the new server (smoke test in CI).
- All 12 site pages render and behave equivalently (auth, navigation, modals, forms).
- Postgres → SQLite import preserves every row of the user's project, verifiable by a per-table count + sanity-join check.
- `docker compose up` brings up `caddy` + `acai` (only); SQLite file lives in a named volume; Litestream replicates to S3.
- Cutover procedure documented; rollback path exists for at least one week post-cutover.

---

## 2. Architecture & stack

### Shape

One Go binary (`acai`) listens on a single HTTP port behind Caddy. Routes split into:

- `/api/v1/*` — REST API for the CLI (bearer auth, OpenAPI 3.1, rate-limited)
- `/*` — site pages rendered with `templ`, made reactive via Datastar SSE
- `/sse/team/{team_id}/events` — long-lived SSE stream for live updates triggered by API push
- `/_health`, `/admin/*` — ops

SQLite is the only datastore. WAL mode, single-writer pool + multi-reader pool. Static assets, templates, and migrations are embedded with `embed.FS` so deployment is one binary + one DB file.

### Stack picks

| Concern | Choice | Rationale |
|---|---|---|
| HTTP router | `github.com/go-chi/chi/v5` | Idiomatic, stable, middleware-composable |
| OpenAPI gen | `github.com/danielgtaylor/huma/v2` (chi adapter) | Generates OpenAPI 3.1 from typed Go handlers; reflects the CLI contract from code |
| SQLite driver | `modernc.org/sqlite` (pure Go) | No CGO, trivial cross-compile, single static binary; JSON1 included |
| SQL layer | `github.com/sqlc-dev/sqlc` | Compile-time-checked SQL → typed Go |
| Migrations | `github.com/pressly/goose/v3` | SQL-file migrations, embeddable, runs on startup |
| Templates | `github.com/a-h/templ` | Compile-time-checked HTML, type-safe components, Datastar's natural partner |
| Datastar SDK | `github.com/starfederation/datastar/sdk/go` | Official Go SDK: SSE patching, signal handling, `data-*` directives |
| Sessions | `github.com/alexedwards/scs/v2` + `scs/sqlitestore` | Cookie-signed sessions backed by SQLite |
| Password hashing | `golang.org/x/crypto/argon2` (Argon2id) | Matches `argon2_elixir` on-disk hash format → legacy hashes verify without rehashing |
| Email (Mailgun) | `github.com/mailgun/mailgun-go/v4` (or stdlib `net/smtp`) | Same SMTP target as today |
| TLS / proxy | Caddy (unchanged container) | Auto-TLS for self-host |
| Backup | `github.com/benbjohnson/litestream` (embedded as goroutine) | Continuous WAL → S3 |
| Logging | `log/slog` with **`slog.NewJSONHandler` only — JSON output everywhere, dev and prod** | Stdlib, structured, machine-readable |
| Tests | stdlib `testing`, `httptest`, `github.com/stretchr/testify` | SQLite in-process; no Docker for tests |
| CSRF | `github.com/gorilla/csrf` | Cookie-based, scoped to browser routes |
| Rate-limit storage | in-process `sync.Map` (interface for swap) | Single-instance self-host; matches existing ETS design |

### Kept from current
- Caddy (TLS + reverse proxy)
- Devcontainer / DevPod parallel-instance pattern (simpler — no Postgres container)
- `features/*.feature.yaml` spec discipline + spec-ID code comments (`core.ENG.6`, `push.ENDPOINT.1`, etc.)

### Removed
- Postgres + Restic
- BEAM runtime, OTP supervision, Phoenix.PubSub, LiveView's stateful per-client process model
- `pg_stat_statements`, `ecto_psql_extras`

### Key open calls (made, ready to revisit)
- **`modernc.org/sqlite` over `mattn/go-sqlite3`** — pure Go for cross-compile simplicity. One-line driver swap if perf demands it later.
- **`huma` over `oapi-codegen`** — code-first OpenAPI keeps types and spec from drifting.
- **`templ` over `html/template`** — type-safe templates compile to Go, integrate naturally with Datastar fragment rendering.
- **Embedded Litestream** over sidecar — preserves "one binary" pitch; `recover()` around the goroutine; sidecar is a future option.
- **DaisyUI v5 stays** despite AGENTS.md "no DaisyUI" guidance — current code uses it; stripping is a separate visual rewrite.

---

## 3. Project layout

```
acai/
├── cmd/acai/main.go                # single binary entrypoint + subcommands
├── internal/
│   ├── api/                        # /api/v1 — huma operations
│   │   ├── push.go                 # POST  /api/v1/push
│   │   ├── feature_states.go       # PATCH /api/v1/feature-states
│   │   ├── implementations.go      # GET   /api/v1/implementations
│   │   ├── feature_context.go      # GET   /api/v1/feature-context
│   │   ├── implementation_features.go
│   │   ├── auth.go                 # bearer-token middleware
│   │   ├── ratelimit.go
│   │   ├── operations.go           # operation-config / size caps
│   │   ├── errors.go               # apierror.Write — preserves Phoenix error envelope
│   │   └── schemas/                # ports of lib/acai_web/api/schemas/*.ex
│   ├── site/
│   │   ├── pages/      *.templ     # one per LiveView page
│   │   ├── components/ *.templ     # nav, feature_settings, feature_status_dropdown, ...
│   │   ├── layouts/    *.templ     # app/root layout
│   │   ├── handlers/   *.go        # request → data → templ component
│   │   ├── datastar.go             # SSE helpers (patch fragment, signal updates)
│   │   └── router.go
│   ├── auth/
│   │   ├── session.go              # scs config + sqlite store
│   │   ├── magic_link.go
│   │   ├── scope.go                # Scope = {*User}
│   │   ├── password.go             # argon2id, with legacy-hash verify
│   │   ├── tokens.go               # API bearer tokens
│   │   └── middleware.go           # LoadScope, RequireAuth, RequireSudo, RequireGlobalAdmin, RedirectIfAuth
│   ├── domain/                     # business contexts (≡ Phoenix contexts)
│   │   ├── accounts/               # User, EmailToken, Scope
│   │   ├── teams/                  # Team, UserTeamRole, AccessToken, Permissions
│   │   ├── products/
│   │   ├── implementations/        # Implementation, Branch, TrackedBranch
│   │   ├── specs/                  # Spec, FeatureBranchRef, FeatureImplState
│   │   └── core/                   # shared validations
│   ├── services/                   # ≡ lib/acai/services
│   │   ├── push.go
│   │   └── feature_states.go
│   ├── store/
│   │   ├── migrations/  *.sql      # goose-formatted, embed.FS
│   │   ├── queries/     *.sql      # sqlc inputs
│   │   ├── sqlc/        *.go       # sqlc output (committed)
│   │   ├── db.go                   # Open(), WAL, pragmas, busy_timeout
│   │   └── tx.go                   # transaction helper
│   ├── pubsub/                     # in-process pub/sub for live updates
│   ├── mail/                       # ≡ Acai.Mailer + UserNotifier
│   ├── ops/                        # health, telemetry, slog setup
│   ├── config/                     # env → typed Config, validated at boot
│   └── migrate/                    # one-shot Postgres → SQLite tool
│       └── pg_to_sqlite.go
├── assets/
│   ├── css/app.css                 # Tailwind v4 input
│   ├── js/datastar.min.js          # vendored Datastar runtime
│   └── icons/heroicons/            # embedded SVG subset
├── infra/
│   ├── docker-compose.yml          # caddy + acai only
│   ├── caddy/                      # Caddyfile.{prod,devcontainer,devrelease}
│   └── litestream/litestream.yml
├── features/                       # spec yaml — unchanged
├── sqlc.yaml
├── Makefile
└── go.mod / go.sum
```

### Mapping back to Phoenix

| Phoenix | Go |
|---|---|
| `lib/acai/accounts/*.ex` | `internal/domain/accounts/*.go` |
| `lib/acai/teams/*.ex` | `internal/domain/teams/*.go` |
| `lib/acai/{products,implementations,specs,core}` | `internal/domain/{products,implementations,specs,core}` |
| `lib/acai/services/*.ex` | `internal/services/*.go` |
| `lib/acai_web/api/*` + `controllers/api/*` | `internal/api/*` |
| `lib/acai_web/live/*` + `controllers/page_*` | `internal/site/*` |
| `lib/acai_web/user_auth.ex` + `controllers/user_*` | `internal/auth/*` + `internal/site/handlers/auth_*.go` |
| `priv/repo/migrations/*.exs` | `internal/store/migrations/*.sql` |
| `assets/{css,js,icons}` | `assets/{css,js,icons}` |
| `features/*.feature.yaml` | `features/*.feature.yaml` (preserved) |

### Boundary rules

- `internal/store/sqlc/` is generated — only `internal/store/` imports it.
- `internal/domain/<context>` is the only place that calls into the store; handlers call domain, never store directly.
- `internal/site/handlers` and `internal/api` both depend on `internal/domain` — same business logic, different rendering.
- `internal/services/*` exists for cross-context operations (e.g., `push` writes specs + feature-states + implementations).

### Subcommands

- `acai serve` (default) — start the HTTP server + embedded Litestream
- `acai migrate` — run goose migrations and exit
- `acai import-postgres --pg-url=… --out=… [--force] [--verify]` — one-shot Pg→SQLite migration
- `acai create-admin --email=… [--password=…]` — bootstrap the first user (replaces seeds.exs)
- `acai healthcheck` — used by Docker HEALTHCHECK; exits 0 if `/_health` returns ok against localhost
- `acai litestream status` — Litestream generation/replication status
- `acai restore --from-s3 --out=…` — pull latest generation from S3 to a fresh SQLite file

---

## 4. Data layer

### Schema translation: Postgres → SQLite

| Postgres feature | Used by | SQLite equivalent |
|---|---|---|
| `citext` | `users.email`, `teams.name`, `products.name` | `TEXT COLLATE NOCASE` + unique indexes that respect collation |
| `:uuid` (UUIDv7) | every PK & FK | `TEXT` (hyphenated; debuggable via `.dump`) |
| `:utc_datetime` | every `inserted_at`/`updated_at` | `TEXT` ISO-8601, populated in Go (no DB defaults) |
| `:jsonb` | `access_tokens.scopes`, `specs.requirements`, `feature_impl_states.states`, `feature_branch_refs.refs` | `TEXT` with built-in JSON1 functions |
| GIN index on jsonb | `feature_impl_states.states`, `feature_branch_refs.refs` | **Dropped initially.** Both tables accessed by `(implementation_id, feature_name)` / `(branch_id, feature_name)` with unique indexes. Add JSON expression index later if a query pattern demands it. |
| Regex check (`name ~ '^[a-zA-Z0-9_-]+$'`) | several `name` columns | Moved to Go domain validation (source of truth). DB keeps `CHECK (name <> '')` safety net. |
| `:binary` | `users_tokens.token` (renamed `email_tokens.token_hash`) | `BLOB` |
| `pg_stat_statements`, `ecto_psql_extras` | ops-only | Dropped. Replaced with slog query timing + dev-only slow-query logger. |
| `:nilify_all` self-FK on `implementations.parent_implementation_id` | self-referential | `ON DELETE SET NULL` |
| `:delete_all` cascades | most FKs | `ON DELETE CASCADE` |

### Initial migration (`001_initial.sql`)

Squashes the 5 Phoenix migrations into a single fresh-SQLite schema. Final state from migration 5 (with `teams.global_admin` column) is the starting state. Future schema changes get sequential goose migrations.

### Tables (SQLite DDL summary)

```sql
CREATE TABLE users (
  id              TEXT PRIMARY KEY,
  email           TEXT NOT NULL COLLATE NOCASE,
  hashed_password TEXT,
  confirmed_at    TEXT,
  authenticated_at TEXT,
  inserted_at     TEXT NOT NULL,
  updated_at      TEXT NOT NULL
);
CREATE UNIQUE INDEX users_email_idx ON users(email COLLATE NOCASE);

CREATE TABLE email_tokens (   -- renamed from users_tokens; sessions moved to scs
  id          TEXT PRIMARY KEY,
  user_id     TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
  token_hash  BLOB NOT NULL,
  context     TEXT NOT NULL,            -- 'login' | 'change_email:<old>'
  sent_to     TEXT NOT NULL,
  inserted_at TEXT NOT NULL,
  UNIQUE(context, token_hash)
);
CREATE INDEX email_tokens_user_idx ON email_tokens(user_id);

CREATE TABLE teams (
  id           TEXT PRIMARY KEY,
  name         TEXT NOT NULL COLLATE NOCASE,
  global_admin INTEGER NOT NULL DEFAULT 0,   -- migration 5
  inserted_at  TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  CHECK (name <> '')
);
CREATE UNIQUE INDEX teams_name_idx ON teams(name COLLATE NOCASE);

CREATE TABLE user_team_roles (
  team_id     TEXT NOT NULL REFERENCES teams ON DELETE CASCADE,
  user_id     TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
  title       TEXT NOT NULL,
  inserted_at TEXT NOT NULL,
  updated_at  TEXT NOT NULL,
  UNIQUE(team_id, user_id)
);
CREATE INDEX user_team_roles_user_idx ON user_team_roles(user_id);

CREATE TABLE products (
  id          TEXT PRIMARY KEY,
  team_id     TEXT NOT NULL REFERENCES teams ON DELETE CASCADE,
  name        TEXT NOT NULL COLLATE NOCASE,
  description TEXT,
  is_active   INTEGER NOT NULL DEFAULT 1,
  inserted_at TEXT NOT NULL,
  updated_at  TEXT NOT NULL,
  UNIQUE(team_id, name),
  CHECK (name <> '')
);

CREATE TABLE access_tokens (
  id            TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL REFERENCES users ON DELETE CASCADE,
  team_id       TEXT NOT NULL REFERENCES teams ON DELETE CASCADE,
  name          TEXT NOT NULL,
  token_hash    TEXT NOT NULL,
  token_prefix  TEXT NOT NULL,
  scopes        TEXT NOT NULL,           -- JSON
  expires_at    TEXT,
  revoked_at    TEXT,
  last_used_at  TEXT,
  inserted_at   TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);
CREATE UNIQUE INDEX access_tokens_hash_idx ON access_tokens(token_hash);
CREATE INDEX access_tokens_prefix_idx ON access_tokens(token_prefix);
CREATE INDEX access_tokens_user_idx ON access_tokens(user_id);
CREATE INDEX access_tokens_team_idx ON access_tokens(team_id);

CREATE TABLE implementations (
  id                       TEXT PRIMARY KEY,
  product_id               TEXT NOT NULL REFERENCES products ON DELETE CASCADE,
  team_id                  TEXT NOT NULL REFERENCES teams ON DELETE CASCADE,
  parent_implementation_id TEXT REFERENCES implementations ON DELETE SET NULL,
  name                     TEXT NOT NULL,
  description              TEXT,
  is_active                INTEGER NOT NULL DEFAULT 1,
  inserted_at              TEXT NOT NULL,
  updated_at               TEXT NOT NULL,
  UNIQUE(product_id, name)
);
CREATE INDEX implementations_team_idx ON implementations(team_id);
CREATE INDEX implementations_parent_idx ON implementations(parent_implementation_id);

CREATE TABLE branches (
  id                TEXT PRIMARY KEY,
  team_id           TEXT NOT NULL REFERENCES teams ON DELETE CASCADE,
  repo_uri          TEXT NOT NULL,
  branch_name       TEXT NOT NULL,
  last_seen_commit  TEXT NOT NULL,
  inserted_at       TEXT NOT NULL,
  updated_at        TEXT NOT NULL,
  UNIQUE(team_id, repo_uri, branch_name)
);
CREATE INDEX branches_repo_idx ON branches(repo_uri);

CREATE TABLE tracked_branches (
  implementation_id TEXT NOT NULL REFERENCES implementations ON DELETE CASCADE,
  branch_id         TEXT NOT NULL REFERENCES branches ON DELETE CASCADE,
  repo_uri          TEXT NOT NULL,
  inserted_at       TEXT NOT NULL,
  updated_at        TEXT NOT NULL,
  PRIMARY KEY (implementation_id, branch_id),
  UNIQUE(implementation_id, repo_uri)
);
CREATE INDEX tracked_branches_branch_idx ON tracked_branches(branch_id);

CREATE TABLE specs (
  id                  TEXT PRIMARY KEY,
  product_id          TEXT NOT NULL REFERENCES products ON DELETE CASCADE,
  branch_id           TEXT NOT NULL REFERENCES branches ON DELETE CASCADE,
  path                TEXT,
  last_seen_commit    TEXT NOT NULL,
  parsed_at           TEXT NOT NULL,
  feature_name        TEXT NOT NULL,
  feature_description TEXT,
  feature_version     TEXT NOT NULL DEFAULT '1.0.0',
  raw_content         TEXT,
  requirements        TEXT NOT NULL DEFAULT '{}',   -- JSON
  inserted_at         TEXT NOT NULL,
  updated_at          TEXT NOT NULL,
  UNIQUE(branch_id, feature_name),
  CHECK (feature_name <> '')
);
CREATE INDEX specs_product_idx ON specs(product_id);
CREATE INDEX specs_branch_idx ON specs(branch_id);

CREATE TABLE feature_impl_states (
  id                TEXT PRIMARY KEY,
  implementation_id TEXT NOT NULL REFERENCES implementations ON DELETE CASCADE,
  feature_name      TEXT NOT NULL,
  states            TEXT NOT NULL DEFAULT '{}',   -- JSON
  inserted_at       TEXT NOT NULL,
  updated_at        TEXT NOT NULL,
  UNIQUE(implementation_id, feature_name),
  CHECK (feature_name <> '')
);
CREATE INDEX feature_impl_states_impl_idx ON feature_impl_states(implementation_id);

CREATE TABLE feature_branch_refs (
  id           TEXT PRIMARY KEY,
  branch_id    TEXT NOT NULL REFERENCES branches ON DELETE CASCADE,
  feature_name TEXT NOT NULL,
  refs         TEXT NOT NULL DEFAULT '{}',   -- JSON
  commit       TEXT NOT NULL,
  pushed_at    TEXT NOT NULL,
  inserted_at  TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  UNIQUE(branch_id, feature_name),
  CHECK (feature_name <> '')
);
CREATE INDEX feature_branch_refs_branch_idx ON feature_branch_refs(branch_id);
```

### Connection setup

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;
PRAGMA temp_store = MEMORY;
PRAGMA cache_size = -20000;     -- ~20 MB
```

### Pool shape

- Two `*sql.DB` handles against the same file:
  - **Write pool**: `SetMaxOpenConns(1)` — serializes writers, eliminates `SQLITE_BUSY`
  - **Read pool**: `SetMaxOpenConns(runtime.NumCPU()*2)` — WAL allows many concurrent readers
- `internal/store.Store` wraps both; mutating callers use `store.WriteTx(ctx, fn)`, reads use `store.ReadConn()`.

### Postgres → SQLite import (`acai import-postgres`)

1. Open Postgres via `pgx` (read-only).
2. Create fresh SQLite at `--out`, run goose migrations to head.
3. One big SQLite transaction with `PRAGMA defer_foreign_keys = ON`:
   - Stream each table in FK order: users → email_tokens (skip session-context rows) → teams → user_team_roles → products → access_tokens → branches → implementations → tracked_branches → specs → feature_impl_states → feature_branch_refs.
   - Use `pgx.Rows` cursor, batch 1000 rows per multi-row `INSERT`.
4. Verification pass (`--verify`): row counts, sanity joins, all UUIDs/timestamps/JSON parseable.
5. Print summary; refuse to overwrite without `--force`.

Value conversions:
- `pgx.Time` → RFC3339 string
- pg UUID → hyphenated string
- pg `[]byte` (jsonb) → string (re-emitted via `json.Compact`)
- pg `[]byte` (binary tokens) → SQLite BLOB
- pg bool → `int64(0|1)`
- citext → string

Session-context rows from `users_tokens` are **skipped** during import (sessions live in scs, not the email_tokens table). Login/change_email tokens are imported. (Effectively: filter `WHERE context != 'session'`.)

### Out of scope
- Live Pg/SQLite dual-write
- "Downgrade back to Postgres" path
- Postgres-specific ops tooling

---

## 5. API parity (preserving the CLI contract)

### Endpoint inventory (preserved 1:1)

| Method | Path | Auth | Spec ID |
|---|---|---|---|
| GET | `/api/v1/openapi.json` | public | core.API.1 |
| POST | `/api/v1/push` | bearer | push.ENDPOINT.1 |
| PATCH | `/api/v1/feature-states` | bearer | feature-states.ENDPOINT.1 |
| GET | `/api/v1/implementations` | bearer | implementations.ENDPOINT.1 |
| GET | `/api/v1/feature-context` | bearer | feature-context.ENDPOINT.1 |
| GET | `/api/v1/implementation-features` | bearer | implementation-features.ENDPOINT.1 |

### Request pipeline

```
chi → RequestID/Logger/Recoverer
    → api.BearerAuth     401 missing/malformed/unknown/revoked/expired
    → api.SizeCap        413 from per-endpoint Content-Length cap
    → api.RateLimit      429 keyed on (endpoint, token_id, time-bucket)
    → huma operation     422 spec validation; auto-cast input/output
    → handler            domain.<context> calls; uniform error envelope
```

### Bearer auth

- Token format presented: `Authorization: Bearer ${prefix}.${secret}`
- Prefix lookup via `access_tokens.token_prefix` index → argon2id verify against `token_hash` → check `expires_at`, `revoked_at`
- Result puts `*Token` and `*Team` into `context.Context`
- `last_used_at` updated async via the write pool (failure logged; doesn't block request)

### Rate limiter

Port `RateLimiter` ETS bucket-counter design 1:1: `sync.Map[bucketKey]*atomic.Int64`, pruned on each call when bucket boundary rolls. Single-instance self-host = in-process is sufficient. `Limiter` interface allows SQLite/Redis swap later.

### Operations config

`Config.API.Operations` struct mirrors `runtime.exs` exactly:
- Preserve every env-var name (`API_PUSH_REQUEST_SIZE_CAP`, `API_PUSH_MAX_SPECS`, `API_PUSH_RATE_LIMIT_WINDOW_SECONDS`, etc.) so existing `.env` files copy with zero edits.
- Same defaults for non-prod vs prod (`Config.NonProd bool`).
- Same per-endpoint keys (`default`, `push`, `feature_states`).

### Schema preservation

Each `lib/acai_web/api/schemas/*.ex` ports to a Go file in `internal/api/schemas/`:

```
push_schemas.ex          → internal/api/schemas/push.go
feature_states_schemas.ex → internal/api/schemas/feature_states.go
read_schemas.ex          → internal/api/schemas/read.go
```

Huma struct tags preserve every field name, JSON type, default, `maxLength`, `maxItems`, `maxProperties`, `additionalProperties: false`, required marker, nullability, and example. Schemas reference by Go type → `allOf`/`$ref` composition stays intact.

### Error envelope

Two shapes coexist today:

```json
// app-level errors (auth, size, rate, business-logic): single object
{"errors": {"detail": "Token revoked", "status": "UNAUTHORIZED"}}

// validation errors (open_api_spex v2): array of structured items
{"errors": [{"title": "Invalid value", "source": {"pointer": "/specs/0/feature/name"}, ...}]}
```

A central `apierror.Write(w, kind, args...)` helper picks the right envelope per `kind`. Huma's default error formatter is replaced with a custom one that emits the exact `json_render_error_v2: true` shape. Per-endpoint goldens with crafted invalid payloads enforce this.

### OpenAPI spec golden test

1. Capture current Phoenix `/api/v1/openapi.json` once → `testdata/openapi.golden.json`.
2. Build until `huma`'s emitted spec matches semantically (paths, operations, schemas, parameters, security scheme, examples, descriptions).
3. `TestOpenAPISpecParity` does semantic diff; expected drift documented in `testdata/openapi.diff.json`.

### CLI smoke test

CI step boots an in-memory test instance with seeded data and runs the actual CLI binary against each verb (happy path + error cases). Catches behavior drift the spec golden can't see. Sub-second per case.

### RejectionLog port

Port to `slog` JSON output with same field names: `endpoint`, `request_size`, `request_size_cap`, `request_count`, `request_limit`, `window_seconds`, `token_fingerprint = sha256(token)[0:12]`. Observability, not contract.

---

## 6. Frontend & Datastar

### Page mapping

| LiveView | Datastar page |
|---|---|
| `TeamsLive` | `pages/teams.templ` + `handlers/teams.go` |
| `TeamLive` | `pages/team.templ` |
| `ProductLive` | `pages/product.templ` |
| `FeatureLive` | `pages/feature.templ` |
| `ImplementationLive` | `pages/implementation.templ` |
| `TeamSettingsLive` | `pages/team_settings.templ` |
| `TeamTokensLive` | `pages/team_tokens.templ` |
| `NavLive`, `FeatureSettingsLive`, `ImplementationSettingsLive`, `RequirementDetailsLive`, `FeatureStatusDropdown` | `components/*.templ` |

### Interaction translation

| LiveView | Datastar |
|---|---|
| Socket assigns | URL state (path/query) is source of truth; minimal client signals via `data-signals` for transient UI |
| `phx-click="event"` | `data-on-click="@post('/path')"` → server returns SSE fragments |
| `phx-submit` + `phx-change` | HTML form + `data-on-input__debounce.300ms="@post('/x/validate')"`; submit either redirects (`datastar-redirect` SSE event) or patches form-error spans |
| `stream(:items, list)` | Server-render full list; for incremental updates emit `datastar-merge-fragments` with `merge=append/prepend/upsert/replace` |
| `push_navigate` | `<a href>` for normal nav; `data-on-click="@get('/path', {history: 'push'})"` only where SPA-feel is worth it |
| Modal via `@show_modal` | Pure client signal: `data-signals="{showModal: false}"` + `data-show="$showModal"` |
| Phoenix.PubSub broadcast | `internal/pubsub` (`sync.Map[teamID]chan event`) + `/sse/team/{team_id}/events` endpoint |

### Where SSE-pushed real-time matters

For a single-user self-host, only one place earns real-time updates: when an agent calls `/api/v1/push` while the user has the page open. Implementation:

- API push handler publishes to `internal/pubsub` after commit
- `/sse/team/{team_id}/events` (auth via session cookie) holds a long-lived SSE connection per browser tab
- SSE handler renders the affected fragment via templ → emits `datastar-merge-fragments` with target DOM ID
- Pages with no live-update need (settings, token mgmt) don't open this connection

### Tailwind v4, DaisyUI, heroicons

- **DaisyUI v5 stays.** Current LiveViews use `card`, `btn`, `card-body`, `bg-base-100`, `border-base-300`, etc. Stripping is a separate visual rewrite, out of scope here.
- **Heroicons** embedded under `assets/icons/heroicons/` as raw SVGs; templ helper `Icon("plus", "size-4 mr-1")` interpolates inline.
- **Datastar runtime** vendored at `assets/js/datastar.min.js`. One `<script type="module">` in layout. No JS bundler.

### Build pipeline

| Tool | Output | When |
|---|---|---|
| `templ generate` | `*_templ.go` next to `*.templ` (committed; CI verifies fresh) | Pre-build |
| `tailwindcss -i assets/css/app.css -o assets/dist/app.css --minify` | CSS (embedded) | Pre-build |
| `sqlc generate` | `internal/store/sqlc/*.go` (committed; CI verifies fresh) | Pre-build |
| `go build` | `acai` binary with embedded `assets/dist/*`, `assets/icons/*`, `internal/store/migrations/*` | Build |

### Dev loop

`make dev` runs three watchers concurrently: `templ generate --watch`, `tailwindcss --watch`, `air` (Go rebuild + restart). In dev the binary reads templates fresh from disk (`--reload-templates`); in prod everything is embedded.

### Hierarchy / status-percentage logic

Pure data (parent-child ordering capped at depth 4, status percent calc). Ports to `BuildImplCards(impls, counts, specReqCounts) []ImplCardVM` consumed by `feature.templ`. SSE updates re-render a single card and merge it.

### Layouts & flash

`Layouts.app` HEEx → `layouts/app.templ` with same props (`flash`, `currentScope`). Flash group uses `data-signals="{visible: true}"` + `data-on-click__delay.5000ms="$visible=false"` for auto-dismiss.

---

## 7. Auth

### Sessions (browser)

`alexedwards/scs/v2` + `scs/sqlitestore`.

| Phoenix today | Go |
|---|---|
| `users_tokens` row context=session, raw token in DB | scs row in `sessions` table keyed by random session ID |
| Session cookie + duplicate `_acai_web_user_remember_me` cookie | One scs cookie, `Persist=true` if remember-me else session-lifetime |
| Token reissue every 7 days | `sessionManager.RenewToken(ctx)` on the same cadence |
| `delete_csrf_token` on session renew | scs renews session ID; gorilla/csrf rotates its token |
| Session validity 14 days | `sessionManager.Lifetime = 14 * 24h` |
| `live_socket_id` broadcast disconnect | Not needed (Datastar SSE reconnects) |

Session payload: `{user_id uuid, authenticated_at time.Time}`.
Cookie: name `_acai_session`, `HttpOnly`, `Secure` in prod, `SameSite=Lax`, signed by `SECRET_KEY_BASE`.

### Magic-link & email-confirm tokens

Renamed `users_tokens` → `email_tokens` (sessions moved to scs). Schema in §4.

- Generation: `token = crypto/rand 32 bytes`; `tokenHash = sha256(token)` stored as BLOB; `urlToken = base64.URLEncoding.WithoutPadding(token)` sent in email
- Verification: re-derive `sha256(urlToken)`, lookup by `(context, hash)`, check `inserted_at` within validity window
- Validity: `login` 15 min; `change_email:*` 7 days
- Single-use: row deleted in verifying transaction

### Sudo mode

20-min window. Stored on session: `authenticated_at` updated to `time.Now()` after password verify or magic-link confirm. Middleware: `now.Sub(scope.AuthenticatedAt) < 20m`.

### Global admin

`teams.global_admin = 1` row that user belongs to. Lookup in `internal/domain/teams/permissions.go: IsGlobalAdmin(ctx, scope)`. Middleware on `/admin/*` only.

### Password hashing

`golang.org/x/crypto/argon2.IDKey` with `argon2_elixir` defaults: `time=3, memory=64*1024 KiB, parallelism=4, keyLen=32, saltLen=16`. Output format identical: `$argon2id$v=19$m=65536,t=3,p=4$<saltB64>$<hashB64>`. Legacy hashes from imported Postgres data verify without rehashing.

### CSRF

`gorilla/csrf` middleware on browser routes (skipped on `/api/v1/*` and `/sse/*`). Templ helper `csrf.TokenInput(ctx)` injects hidden input. Datastar `data-on-submit` POSTs include the hidden input via Datastar's form-include mechanism.

### Bearer tokens (API)

`access_tokens` table preserved. Token format `${prefix}.${secret}`. Verification path described in §5 (Bearer auth).

### Route placement

```go
r.Use(slog_chi.Middleware, requestid.Middleware, recoverer.Middleware, sessionManager.LoadAndSave)
r.Use(auth.LoadScope)

r.Group(func(r chi.Router) { // public (browser)
    r.Use(csrf.Middleware)
    r.Get("/", site.Home)
    r.Get("/_health", ops.Health)
})

r.Group(func(r chi.Router) { // require NOT authed
    r.Use(csrf.Middleware, auth.RedirectIfAuth)
    r.Get("/users/register", site.RegisterNew)
    r.Post("/users/register", site.RegisterCreate)
    r.Get("/users/log-in", site.LoginNew)
    r.Get("/users/log-in/{token}", site.LoginConfirm)
    r.Post("/users/log-in", site.LoginCreate)
})

r.Group(func(r chi.Router) { // require authed
    r.Use(csrf.Middleware, auth.RequireAuth)
    r.Get("/teams", site.TeamsIndex)
    r.Get("/t/{team_name}", site.TeamShow)
    r.Get("/t/{team_name}/p/{product_name}", site.ProductShow)
    r.Get("/t/{team_name}/f/{feature_name}", site.FeatureShow)
    r.Get("/t/{team_name}/i/{impl_slug}/f/{feature_name}", site.ImplementationShow)
    r.Get("/t/{team_name}/settings", site.TeamSettings)
    r.Get("/t/{team_name}/tokens", site.TeamTokens)
    r.Get("/users/settings", site.UserSettings)
    r.Put("/users/settings", site.UserSettingsUpdate)
    r.Get("/users/settings/confirm-email/{token}", site.ConfirmEmail)
    r.Get("/admin/dashboard", auth.RequireSudo(auth.RequireGlobalAdmin(site.AdminDashboard)))
    r.Delete("/users/log-out", site.LogOut)
})

r.Route("/api/v1", func(r chi.Router) {
    r.Get("/openapi.json", api.OpenAPISpec)
    r.Group(func(r chi.Router) {
        r.Use(api.BearerAuth, api.SizeCap, api.RateLimit)
        api.Mount(r) // huma operations
    })
})

r.Get("/sse/team/{team_id}/events", auth.RequireAuth(sse.TeamEvents))
```

Each guard carries its existing spec ID in code comments (`team-list.MAIN.3`, `dashboard.AUTH.2`, etc.).

---

## 8. Infra & deployment

### docker-compose: 5 services → 2

| Today | Tomorrow |
|---|---|
| `caddy` | unchanged |
| `app` (Phoenix release) | `acai` (Go binary, distroless) |
| `db` (Postgres 17) | gone |
| `backup` (Restic + cron) | gone (replaced by embedded Litestream) |
| `devcontainer` | unchanged shape; Go dev image, no DB dep |

```yaml
services:
  caddy:    # unchanged
  acai:
    image: ghcr.io/acai-sh/server:${IMAGE_TAG_VERSION:-latest}
    restart: unless-stopped
    volumes:
      - acai_data:/data
    environment:
      DATABASE_PATH: /data/acai.db
      HTTP_PORT: ${HTTP_PORT_INTERNAL:-4000}
      SECRET_KEY_BASE: ${SECRET_KEY_BASE:-}
      URL_HOST, URL_PATH, URL_PORT, URL_SCHEME: ...
      MAILGUN_API_KEY, MAILGUN_DOMAIN, MAILGUN_BASE_URL, MAIL_FROM_*: ...
      LITESTREAM_S3_BUCKET, LITESTREAM_S3_REGION, LITESTREAM_S3_ENDPOINT, LITESTREAM_S3_PATH: ...
      AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY: ...
      LOG_LEVEL: ${LOG_LEVEL:-info}
    healthcheck:
      test: ["CMD", "/acai", "healthcheck"]
volumes:
  caddy_data:
  caddy_config:
  acai_data:
```

Resource budget for `acai`: ~256M reservation (down from 2.5G).

### Litestream

**Embedded as a goroutine** inside the acai binary via Litestream's Go API. Disabled if `LITESTREAM_S3_BUCKET` unset (dev). `recover()` around the goroutine; restart with backoff on panic. `acai litestream status` exposes generation/replication state. `acai restore --from-s3 --out=…` pulls latest generation to a fresh file.

### Dockerfile

Two-stage, distroless final:

```dockerfile
FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make build

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /src/acai /acai
ENV DATABASE_PATH=/data/acai.db
USER nonroot
ENTRYPOINT ["/acai"]
CMD ["serve"]
```

Final image ~30 MB.

### Caddyfile

Reverse proxies to `acai:4000` instead of `app:4000`. `_caddy` health endpoint preserved. New `/_health` endpoint served by acai directly.

### Devcontainer / DevPod

- Go 1.22 image with `templ`, `sqlc`, `goose`, `tailwindcss-cli-standalone` baked in
- No `db` dependency. SQLite at `/data/acai.db` in a named volume
- `INSTANCE_NAME` namespacing for parallel devcontainers (cleaner — no port conflicts)
- `MAIL_NOOP=true` in dev (mailer logs to slog)
- Optional `mailpit` service via `compose --profile mail up`

### Env-var migration table

| Removed | Reason |
|---|---|
| `DATABASE_URL` | replaced by `DATABASE_PATH` |
| `POOL_SIZE` | SQLite pool config is internal |
| `START_SERVER` | binary always serves with `serve` subcommand |
| `MIX_ENV` | not applicable |
| `RESTIC_*` | replaced by `LITESTREAM_*` + `AWS_*` |
| `POSTGRES_*` | gone |

| Added |
|---|
| `DATABASE_PATH` (default `/data/acai.db`) |
| `LITESTREAM_S3_BUCKET`, `LITESTREAM_S3_REGION`, `LITESTREAM_S3_ENDPOINT`, `LITESTREAM_S3_PATH` |
| `MAIL_NOOP` |
| `LOG_LEVEL` |

| Renamed |
|---|
| `PHX_PORT` → `HTTP_PORT` |

Everything else preserved byte-for-byte.

### CI

GitHub Actions matrix:
- `golangci-lint run`
- Generate verify: `templ generate && sqlc generate && git diff --exit-code`
- `go test -race ./...`
- `go test ./internal/api -run TestOpenAPISpecParity`
- `make cli-smoke`
- Cross-compile `linux/amd64` + `linux/arm64`, push to `ghcr.io/acai-sh/server`

---

## 9. Testing strategy

### Layer 1 — Unit tests

`*_test.go` next to source. Domain logic, validation, hashing, token generation, hierarchy ordering, status-percentage calc. Pure Go, no DB. 100% coverage of `internal/domain/**` and `internal/auth/**` non-glue code.

### Layer 2 — Integration tests (handler + DB)

`httptest.NewServer` against in-memory SQLite (`file::memory:?cache=shared`). `newTestApp(t)` helper:
1. Opens private `:memory:` DB
2. Runs goose migrations (embedded)
3. Returns `*App` with `*chi.Mux`, scs session manager, in-memory pubsub, slog → `bytes.Buffer`
4. Seeds baseline (one user, one team, one product, one access token)
5. `t.Cleanup` for goroutine cleanup

### Layer 3 — API contract / OpenAPI golden

- `testdata/openapi.golden.json` (captured from current Phoenix once)
- `TestOpenAPISpecParity` semantic JSON diff
- Per-endpoint request/response goldens for happy path + 401, 413, 422, 429 errors

### Layer 4 — CLI smoke

`make cli-smoke`:
1. Builds `acai serve`
2. Boots in-memory mode with seeded data
3. Runs the actual CLI binary against each verb (happy path + error cases)
4. Diffs CLI stdout/stderr against goldens

### Layer 5 — Datastar handler tests

Per site handler:
- Initial GET returns expected templ-rendered HTML (parsed via `golang.org/x/net/html`, asserted on DOM nodes — not snapshot-fragile)
- POST endpoints emit expected SSE events (`data:` lines parsed; `datastar-merge-fragments` payloads validated)
- Auth gates redirect or 401 as expected

No browser tests (no Playwright). Datastar is server-driven; handler tests capture the contract.

### Layer 6 — Migration verification

`acai import-postgres … --verify`:
- Per-table row counts match source
- Sample joins resolve
- All UUIDs/timestamps/JSON parseable

### Conventions

- Avoid sleeps; use channel synchronization
- 100% coverage; one test file per source file (per AGENTS.md)
- `testify` assertions
- Race detector: `go test -race ./...`
- One in-memory SQLite per test for isolation

### `make precommit`

```
go vet ./...
golangci-lint run
templ generate && sqlc generate && git diff --exit-code
go test -race ./...
make cli-smoke
```

Pre-commit hook calls this. Mirrors `mix precommit` discipline.

---

## 10. Cutover plan

### Phases

| Phase | Goal | Done when |
|---|---|---|
| **P0 — Repo setup** | New branch, Go scaffolding, CI green | `make precommit` passes on empty skeleton; `openapi.golden.json` captured from current Phoenix |
| **P1 — Foundations** | DB layer, auth, magic-link, scaffolding | A user can sign up + log in via magic-link in dev; sessions persist; password hashes verify against imported argon2 hashes |
| **P2 — API parity** | All 5 `/api/v1/*` endpoints + OpenAPI spec | `TestOpenAPISpecParity` green; `make cli-smoke` green for all verbs |
| **P3 — Site & Datastar** | All site pages + components | All 12 LiveViews ported; `_health` returns ok; each page passes its handler test |
| **P4 — Infra cutover** | Production switch | New docker-compose deployed; `acai import-postgres` migrated real data; old stack decommissioned; Litestream verified streaming to S3 |

### P3 page port order

Most-shared first:
1. Layouts (`app.templ`) + flash component
2. `NavLive` → `nav.templ`
3. `TeamsLive` → `teams.templ`
4. `TeamLive` → `team.templ`
5. `ProductLive` → `product.templ`
6. `FeatureLive` → `feature.templ` (most complex)
7. `ImplementationLive` → `implementation.templ`
8. `TeamSettingsLive`, `TeamTokensLive`
9. Inline components
10. Auth pages (register, log-in, settings)

### Cutover day procedure

```
# 1. On the production VPS:
docker compose down
mv infra/docker-compose.yml infra/docker-compose.phoenix.yml.bak
git pull   # new docker-compose.yml

# 2. Migrate the data (still has access to old Postgres on its volume):
docker compose up -d db
docker compose run --rm acai \
  acai import-postgres \
    --pg-url="$DATABASE_URL" \
    --out=/data/acai.db \
    --verify

# 3. Stop Postgres permanently:
docker compose stop db
docker compose rm -f db

# 4. Bring up the new stack:
docker compose up -d caddy acai

# 5. Verify:
curl -fsS https://app.acai.sh/_caddy
curl -fsS https://app.acai.sh/_health | jq
docker compose logs -f acai | head -50

# 6. Optional: keep the postgres_data volume for a rollback window before deleting it.
```

### Rollback

Within rollback window:
1. `docker compose down`
2. Restore `infra/docker-compose.phoenix.yml.bak` → `docker-compose.yml`
3. `docker compose up -d` (Postgres volume still intact)

After rollback window: discard `postgres_data` volume; canonical truth is `acai_data` + Litestream S3 generations.

### Risk register

| Risk | Mitigation |
|---|---|
| OpenAPI spec drift breaks the CLI | Golden test in CI; CLI smoke test gates merges |
| Magic-link emails fail in prod | Pre-cutover: send test email from new binary using prod Mailgun creds |
| Litestream not actually streaming | `acai litestream status` immediately post-cutover; heartbeat URL alarm |
| Datastar page-load regression vs LiveView | Bench `wrk -t4 -c100 -d10 https://staging/teams` before/after; explicit success criterion |
| SQLite single-writer contention under push burst | Load test 100 parallel `/api/v1/push` with different access tokens; if contention, add WAL checkpoint tuning + write-queue middleware |

---

## 11. Open items and deferred decisions

These do not block the rewrite but are explicitly deferred:

- **Strip DaisyUI per AGENTS.md guidance.** Out of scope here; separate visual rewrite.
- **JSON expression indexes for `feature_impl_states.states` / `feature_branch_refs.refs`.** Add only if a query pattern demands key-into-JSON lookup.
- **Pure-Go SQLite → CGO `mattn` driver swap.** Only if perf measurement demands it.
- **Litestream sidecar instead of embedded.** Only if embedded turns out to interfere with the main process.
- **Browser tests (Playwright).** Not part of the initial test layers.
- **Multi-instance scaling.** Not designed for. SQLite single-writer + in-process pubsub assumes single-instance deployment.
- **Stripping `users_tokens` session-context rows from imported data** — handled via filter, but worth a smoke check on first import that no session tokens leaked through.

---

## 12. Definition of done

- All 5 `/api/v1/*` endpoints green against captured OpenAPI golden + CLI smoke tests
- All 12 site pages ported, each with handler tests passing
- `acai import-postgres` runs cleanly against the user's production Postgres dump with `--verify` green
- New docker-compose deploys to staging; Litestream confirmed streaming to S3
- One full cutover rehearsal on staging with `acai restore --from-s3` validated
- Production cutover executed; `_health` and `_caddy` endpoints green
- One-week rollback window observed without issues
- Old Phoenix stack decommissioned; `postgres_data` volume archived then deleted
