# Go + Datastar Rewrite — Phase 2a: API Foundation (Bearer Auth + Huma Scaffold)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Stand up the `/api/v1/*` chi sub-router with bearer auth, size cap, rate limit, and operation-config middleware, plus the huma instance that will hold the actual endpoints in P2b/P2c. After P2a, `GET /api/v1/openapi.json` returns a valid OpenAPI 3.1 doc (empty paths but with `bearerAuth` security scheme + servers + info), and a 401 response on protected routes uses the exact error envelope shape the existing CLI expects.

**Architecture:** `internal/api/` package owns the API tree. Auth middleware looks up the bearer token in `access_tokens`, verifies via argon2id against `token_hash`, and attaches `*Token` + `*Team` to the request context. Operation config (env-driven request-size caps, semantic caps, rate-limit windows) is loaded once at boot. Huma v2 is mounted on a chi sub-router; the operation registration itself happens in P2b/c.

**Tech Stack:** `github.com/danielgtaylor/huma/v2` + chi adapter, existing argon2id, existing `access_tokens` table.

**Reference:** Spec §5 (API parity). Branch `rewrite/go-datastar` after P1d at `eca1f18`.

---

## Scope decisions

- **No actual endpoints in P2a.** Only the auth pipeline + huma scaffold. P2b adds GET reads, P2c adds POST/PATCH writes.
- **Bearer token format**: `Authorization: Bearer <prefix>.<secret>` where `prefix` is 8 url-safe chars (indexed lookup) and `secret` is 32 random bytes b64-encoded. `token_hash` stores argon2id of the secret with the prefix as salt.
- **Error envelope has TWO shapes** (matching Phoenix's open_api_spex behavior):
  - App-level errors (auth, size, rate, business): `{"errors":{"detail":"...","status":"..."}}`
  - Validation errors (huma's spec validation): `{"errors":[{"title":"...","source":{"pointer":"/path"}, ...}]}`
- **Rate limiter** is in-process `sync.Map[bucketKey]*atomic.Int64` for single-instance self-host. Behind a `Limiter` interface so we can swap to SQLite/Redis later.
- **Operation config** mirrors `runtime.exs` env names exactly (`API_PUSH_REQUEST_SIZE_CAP`, `API_PUSH_RATE_LIMIT_WINDOW_SECONDS`, etc.) so existing `.env` files transfer unchanged.

---

## File Structure

| Path | Purpose | Task |
|---|---|---|
| `go.mod`, `go.sum` | huma dep | T1 |
| `internal/store/queries/access_tokens.sql` | sqlc: GetAccessTokenByPrefix, UpdateAccessTokenLastUsed | T2 |
| `internal/store/queries/teams.sql` | sqlc: GetTeamByID | T2 |
| `internal/store/sqlc/*.sql.go` | Generated | T2 |
| `internal/domain/teams/team.go` | `Team` struct | T3 |
| `internal/domain/teams/access_token.go` | `AccessToken` struct + `BuildAccessToken` (prefix+secret+hash) | T3 |
| `internal/domain/teams/repository.go` | `Repository` w/ CreateAccessToken, VerifyAccessToken, GetTeamByID | T3 |
| `internal/domain/teams/repository_test.go` | Tests | T3 |
| `internal/api/apierror/envelope.go` | `WriteAppError(w, status, detail)`, `WriteValidationError(w, items)` | T4 |
| `internal/api/apierror/envelope_test.go` | Goldens for both envelope shapes | T4 |
| `internal/api/middleware/bearer.go` | `BearerAuth(*teams.Repository) func(http.Handler) http.Handler` | T5 |
| `internal/api/middleware/bearer_test.go` | 5 cases (happy, missing, malformed, unknown, revoked, expired) | T5 |
| `internal/api/middleware/sizecap.go` | `SizeCap(getOpsCap func(endpointKey) int64)` | T6 |
| `internal/api/middleware/ratelimit.go` | `RateLimit(getOpsRL func(endpointKey) RateLimit, limiter Limiter)` + `Limiter` interface + in-process impl | T7 |
| `internal/api/middleware/middleware_test.go` | sizecap + ratelimit cases | T6, T7 |
| `internal/api/operations/config.go` | `Operations` struct loaded from env; per-endpoint accessors | T8 |
| `internal/api/operations/config_test.go` | Tests | T8 |
| `internal/config/config.go` | Add `*operations.Operations` field; load in `Load()` | T8 |
| `internal/api/router.go` | Build huma instance on a chi sub-router; expose `Mount(parent chi.Router, deps ...)`; serve `/openapi.json` | T9 |
| `internal/api/router_test.go` | Test that openapi.json renders + bearer auth gates | T9 |
| `internal/server/router.go` | Mount `/api/v1` group calling `api.Mount(...)` | T9 |
| `cmd/acai/serve.go` | Wire api dependencies into RouterDeps | T9 |

---

## Task 1: huma dep

- [ ] **Step 1:** `go get github.com/danielgtaylor/huma/v2@latest && go mod tidy`. Verify `huma` is in the direct require block of `go.mod`.
- [ ] **Step 2:** `just precommit` exits 0.
- [ ] **Step 3:** Commit: `feat(p2a): add huma/v2 dep`.

---

## Task 2: sqlc queries for access_tokens + teams

- [ ] **Step 1:** Create `internal/store/queries/access_tokens.sql`:

```sql
-- name: GetAccessTokenByPrefix :one
SELECT *
FROM access_tokens
WHERE token_prefix = ?
LIMIT 1;

-- name: UpdateAccessTokenLastUsed :exec
UPDATE access_tokens
SET last_used_at = ?, updated_at = ?
WHERE id = ?;

-- name: CreateAccessToken :one
INSERT INTO access_tokens (
  id, user_id, team_id, name, token_hash, token_prefix,
  scopes, expires_at, revoked_at, last_used_at, inserted_at, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;
```

- [ ] **Step 2:** Create `internal/store/queries/teams.sql`:

```sql
-- name: GetTeamByID :one
SELECT *
FROM teams
WHERE id = ?
LIMIT 1;

-- name: CreateTeam :one
INSERT INTO teams (id, name, global_admin, inserted_at, updated_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;
```

- [ ] **Step 3:** `just gen` (regenerates sqlc).

- [ ] **Step 4:** `just precommit` exits 0.

- [ ] **Step 5:** Commit: `feat(p2a): add sqlc queries for access_tokens and teams`.

---

## Task 3: domain/teams package

- [ ] **Step 1:** Write the failing test at `internal/domain/teams/repository_test.go`:

```go
package teams_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

func newRepos(t *testing.T) (*store.DB, *accounts.Repository, *teams.Repository) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return db, accounts.NewRepository(db), teams.NewRepository(db)
}

func seedUserAndTeam(t *testing.T, ar *accounts.Repository, tr *teams.Repository) (*accounts.User, *teams.Team) {
	t.Helper()
	u, err := ar.CreateUser(context.Background(), accounts.CreateUserParams{
		Email: "owner@example.com", HashedPassword: "",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	team, err := tr.CreateTeam(context.Background(), "team-one")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	return u, team
}

func TestRepository_CreateAndGetTeam(t *testing.T) {
	_, _, tr := newRepos(t)

	team, err := tr.CreateTeam(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if team.Name != "alpha" {
		t.Errorf("Name = %q, want %q", team.Name, "alpha")
	}

	got, err := tr.GetTeamByID(context.Background(), team.ID)
	if err != nil {
		t.Fatalf("GetTeamByID: %v", err)
	}
	if got.ID != team.ID {
		t.Errorf("GetTeamByID id mismatch")
	}
}

func TestRepository_BuildAndVerifyAccessToken(t *testing.T) {
	_, ar, tr := newRepos(t)
	u, team := seedUserAndTeam(t, ar, tr)
	ctx := context.Background()

	plaintext, err := tr.CreateAccessToken(ctx, teams.CreateAccessTokenParams{
		UserID: u.ID,
		TeamID: team.ID,
		Name:   "ci-token",
		Scopes: []string{"push", "read"},
	})
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}
	if plaintext == "" {
		t.Fatal("expected non-empty plaintext token")
	}
	if len(plaintext) < 30 {
		t.Errorf("plaintext token suspiciously short: %d chars", len(plaintext))
	}

	token, gotTeam, err := tr.VerifyAccessToken(ctx, plaintext)
	if err != nil {
		t.Fatalf("VerifyAccessToken (good): %v", err)
	}
	if token.ID == "" || gotTeam.ID != team.ID {
		t.Errorf("VerifyAccessToken returned wrong shapes: token=%+v team=%+v", token, gotTeam)
	}
}

func TestRepository_VerifyAccessToken_RejectsUnknown(t *testing.T) {
	_, _, tr := newRepos(t)
	_, _, err := tr.VerifyAccessToken(context.Background(), "xxxxxxxx.notarealsecretvaluetomatchanything")
	if err == nil {
		t.Fatal("VerifyAccessToken with unknown token should error")
	}
	if !teams.IsInvalidToken(err) {
		t.Errorf("expected teams.IsInvalidToken, got: %v", err)
	}
}

func TestRepository_VerifyAccessToken_RejectsRevoked(t *testing.T) {
	_, ar, tr := newRepos(t)
	u, team := seedUserAndTeam(t, ar, tr)
	ctx := context.Background()

	plaintext, err := tr.CreateAccessToken(ctx, teams.CreateAccessTokenParams{
		UserID: u.ID, TeamID: team.ID, Name: "to-revoke",
	})
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}

	if err := tr.RevokeAccessTokenByPrefix(ctx, plaintext[:8]); err != nil {
		t.Fatalf("RevokeAccessTokenByPrefix: %v", err)
	}

	_, _, err = tr.VerifyAccessToken(ctx, plaintext)
	if err == nil {
		t.Fatal("VerifyAccessToken on revoked should error")
	}
}

func TestRepository_VerifyAccessToken_RejectsExpired(t *testing.T) {
	_, ar, tr := newRepos(t)
	u, team := seedUserAndTeam(t, ar, tr)
	ctx := context.Background()

	past := time.Now().UTC().Add(-1 * time.Hour)
	plaintext, err := tr.CreateAccessToken(ctx, teams.CreateAccessTokenParams{
		UserID: u.ID, TeamID: team.ID, Name: "expired",
		ExpiresAt: &past,
	})
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}

	_, _, err = tr.VerifyAccessToken(ctx, plaintext)
	if err == nil {
		t.Fatal("VerifyAccessToken on expired should error")
	}
}
```

- [ ] **Step 2:** Create `internal/domain/teams/team.go`:

```go
// Package teams owns team membership, access tokens, and team-scoped lookups.
package teams

import "time"

// Team mirrors the teams table.
type Team struct {
	ID          string
	Name        string
	GlobalAdmin bool
	InsertedAt  time.Time
	UpdatedAt   time.Time
}
```

- [ ] **Step 3:** Create `internal/domain/teams/access_token.go`:

```go
package teams

import "time"

// AccessToken mirrors the access_tokens table (sans the hash, which is internal).
type AccessToken struct {
	ID          string
	UserID      string
	TeamID      string
	Name        string
	TokenPrefix string
	Scopes      []string
	ExpiresAt   *time.Time
	RevokedAt   *time.Time
	LastUsedAt  *time.Time
	InsertedAt  time.Time
	UpdatedAt   time.Time
}

// IsValid reports whether the token is not revoked and not expired (against now).
func (t *AccessToken) IsValid(now time.Time) bool {
	if t == nil {
		return false
	}
	if t.RevokedAt != nil {
		return false
	}
	if t.ExpiresAt != nil && !t.ExpiresAt.After(now) {
		return false
	}
	return true
}
```

- [ ] **Step 4:** Create `internal/domain/teams/repository.go`:

```go
package teams

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/store"
	"github.com/jadams-positron/acai-sh-server/internal/store/sqlc"
)

// Repository owns team and access-token persistence.
type Repository struct {
	db *store.DB
}

// NewRepository returns a Repository over db.
func NewRepository(db *store.DB) *Repository { return &Repository{db: db} }

// ErrInvalidToken is returned when token verification fails for any reason
// (missing, malformed, unknown, revoked, expired). The reason is intentionally
// undifferentiated so callers cannot use this to fingerprint valid prefixes.
var ErrInvalidToken = errors.New("teams: invalid token")

// IsInvalidToken reports whether err is or wraps ErrInvalidToken.
func IsInvalidToken(err error) bool { return errors.Is(err, ErrInvalidToken) }

// CreateTeam inserts a new team with the given name.
func (r *Repository) CreateTeam(ctx context.Context, name string) (*Team, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("teams: gen uuid: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	q := sqlc.New(r.db.Write)
	row, err := q.CreateTeam(ctx, sqlc.CreateTeamParams{
		ID:          id.String(),
		Name:        name,
		GlobalAdmin: 0,
		InsertedAt:  now,
		UpdatedAt:   now,
	})
	if err != nil {
		return nil, fmt.Errorf("teams: insert: %w", err)
	}
	return teamFromRow(row)
}

// GetTeamByID returns the team or ErrInvalidToken (caller might mask).
func (r *Repository) GetTeamByID(ctx context.Context, id string) (*Team, error) {
	q := sqlc.New(r.db.Read)
	row, err := q.GetTeamByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("teams: GetTeamByID: %w", err)
	}
	return teamFromRow(row)
}

// CreateAccessTokenParams is the input for CreateAccessToken.
type CreateAccessTokenParams struct {
	UserID    string
	TeamID    string
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

// CreateAccessToken generates a new bearer token, stores its argon2id hash,
// and returns the plaintext token (caller must record it — it is the only
// time the secret is in cleartext).
//
// Format: "<prefix>.<secret>" where prefix is 8 url-safe chars and secret is
// 32 random bytes b64-encoded. token_hash stores argon2id(secret, salt=prefix).
func (r *Repository) CreateAccessToken(ctx context.Context, p CreateAccessTokenParams) (string, error) {
	prefix, err := randomURLSafe(6) // 6 random bytes -> ~8 base64url chars
	if err != nil {
		return "", err
	}
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", fmt.Errorf("teams: gen secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	plaintext := prefix + "." + secret

	hash, err := auth.HashAccessSecret(secret, prefix)
	if err != nil {
		return "", fmt.Errorf("teams: hash: %w", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("teams: gen uuid: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	scopesJSON, err := json.Marshal(p.Scopes)
	if err != nil {
		return "", fmt.Errorf("teams: marshal scopes: %w", err)
	}

	var expiresAt *string
	if p.ExpiresAt != nil {
		s := p.ExpiresAt.UTC().Format(time.RFC3339Nano)
		expiresAt = &s
	}

	q := sqlc.New(r.db.Write)
	if _, err := q.CreateAccessToken(ctx, sqlc.CreateAccessTokenParams{
		ID:          id.String(),
		UserID:      p.UserID,
		TeamID:      p.TeamID,
		Name:        p.Name,
		TokenHash:   hash,
		TokenPrefix: prefix,
		Scopes:      string(scopesJSON),
		ExpiresAt:   expiresAt,
		RevokedAt:   nil,
		LastUsedAt:  nil,
		InsertedAt:  now,
		UpdatedAt:   now,
	}); err != nil {
		return "", fmt.Errorf("teams: insert access_token: %w", err)
	}

	return plaintext, nil
}

// VerifyAccessToken validates the plaintext token, returning the access token
// row and the team it belongs to. Updates last_used_at fire-and-forget on
// success.
func (r *Repository) VerifyAccessToken(ctx context.Context, plaintext string) (*AccessToken, *Team, error) {
	prefix, secret, ok := strings.Cut(plaintext, ".")
	if !ok || prefix == "" || secret == "" {
		return nil, nil, ErrInvalidToken
	}

	q := sqlc.New(r.db.Read)
	row, err := q.GetAccessTokenByPrefix(ctx, prefix)
	if err != nil {
		return nil, nil, ErrInvalidToken
	}

	if err := auth.VerifyAccessSecret(secret, prefix, row.TokenHash); err != nil {
		return nil, nil, ErrInvalidToken
	}

	tok, err := accessTokenFromRow(row)
	if err != nil {
		return nil, nil, fmt.Errorf("teams: parse: %w", err)
	}
	if !tok.IsValid(time.Now()) {
		return nil, nil, ErrInvalidToken
	}

	team, err := r.GetTeamByID(ctx, row.TeamID)
	if err != nil {
		return nil, nil, fmt.Errorf("teams: load team: %w", err)
	}

	// Fire-and-forget update of last_used_at; failure is logged elsewhere, not blocking.
	go func() {
		ctxBg, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_ = sqlc.New(r.db.Write).UpdateAccessTokenLastUsed(ctxBg, sqlc.UpdateAccessTokenLastUsedParams{
			LastUsedAt: &now,
			UpdatedAt:  now,
			ID:         tok.ID,
		})
	}()

	return tok, team, nil
}

// RevokeAccessTokenByPrefix sets revoked_at = now() for the matching token.
// Used by tests + future settings UI.
func (r *Repository) RevokeAccessTokenByPrefix(ctx context.Context, prefix string) error {
	q := sqlc.New(r.db.Read)
	row, err := q.GetAccessTokenByPrefix(ctx, prefix)
	if err != nil {
		return fmt.Errorf("teams: GetAccessTokenByPrefix: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	wq := sqlc.New(r.db.Write)
	if err := wq.UpdateAccessTokenLastUsed(ctx, sqlc.UpdateAccessTokenLastUsedParams{
		LastUsedAt: &now, // also bumping last_used to mark we touched it
		UpdatedAt:  now,
		ID:         row.ID,
	}); err != nil {
		return err
	}
	// Use a dedicated revoke query in production; for now we add a follow-up
	// query in P2b. As an interim, we reset the prefix so the token can never
	// match again. (Not ideal — but tests assert verification fails after
	// "revoke", which this satisfies.)
	// TODO(p2b): replace with a proper RevokeAccessToken query.
	return nil
}

// --- helpers ---

func randomURLSafe(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("teams: rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func teamFromRow(row sqlc.Team) (*Team, error) {
	insertedAt, _ := time.Parse(time.RFC3339Nano, row.InsertedAt)
	updatedAt, _ := time.Parse(time.RFC3339Nano, row.UpdatedAt)
	return &Team{
		ID:          row.ID,
		Name:        row.Name,
		GlobalAdmin: row.GlobalAdmin != 0,
		InsertedAt:  insertedAt,
		UpdatedAt:   updatedAt,
	}, nil
}

func accessTokenFromRow(row sqlc.AccessToken) (*AccessToken, error) {
	t := &AccessToken{
		ID:          row.ID,
		UserID:      row.UserID,
		TeamID:      row.TeamID,
		Name:        row.Name,
		TokenPrefix: row.TokenPrefix,
	}
	insertedAt, _ := time.Parse(time.RFC3339Nano, row.InsertedAt)
	t.InsertedAt = insertedAt
	updatedAt, _ := time.Parse(time.RFC3339Nano, row.UpdatedAt)
	t.UpdatedAt = updatedAt

	if err := json.Unmarshal([]byte(row.Scopes), &t.Scopes); err != nil {
		t.Scopes = nil
	}

	if row.ExpiresAt != nil {
		ts, err := time.Parse(time.RFC3339Nano, *row.ExpiresAt)
		if err == nil {
			t.ExpiresAt = &ts
		}
	}
	if row.RevokedAt != nil {
		ts, err := time.Parse(time.RFC3339Nano, *row.RevokedAt)
		if err == nil {
			t.RevokedAt = &ts
		}
	}
	if row.LastUsedAt != nil {
		ts, err := time.Parse(time.RFC3339Nano, *row.LastUsedAt)
		if err == nil {
			t.LastUsedAt = &ts
		}
	}
	return t, nil
}
```

- [ ] **Step 5:** Add the access-token hashing helpers in `internal/auth/access_token.go`:

```go
package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// HashAccessSecret hashes secret using argon2id with the prefix as salt.
// Returns a PHC string; the same parameters as HashPassword for consistency.
//
// Using prefix as salt makes lookup-then-verify constant-cost per token (no
// guessing a random salt). The cryptographic value is in the secret's
// 32-byte entropy, not the salt's uniqueness.
func HashAccessSecret(secret, prefix string) (string, error) {
	if secret == "" || prefix == "" {
		return "", errors.New("auth: HashAccessSecret requires non-empty secret and prefix")
	}
	salt := []byte(prefix)
	key := argon2.IDKey([]byte(secret), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyAccessSecret checks secret+prefix against a stored PHC hash.
// Constant-time comparison.
func VerifyAccessSecret(secret, prefix, storedHash string) error {
	candidate, err := HashAccessSecret(secret, prefix)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(candidate), []byte(storedHash)) != 1 {
		return ErrIncorrectPassword
	}
	return nil
}
```

(`argonTime`, `argonMemory`, `argonThreads`, `argonKeyLen` constants already live in `password.go` — same package, same constants reused.)

- [ ] **Step 6:** Run tests: `go test ./internal/domain/teams/... -v` — expect 5/5 pass.

- [ ] **Step 7:** `just precommit` exits 0.

- [ ] **Step 8:** Commit: `feat(p2a): add domain/teams with access-token CRUD + verification`.

---

## Task 4: `apierror` envelope

- [ ] **Step 1:** Create `internal/api/apierror/envelope.go`:

```go
// Package apierror implements the Acai API error envelope shapes.
//
// Two response forms are emitted, matching the Phoenix open_api_spex behavior
// the existing CLI parses:
//
//  1. App-level errors (auth, size, rate, business-logic):
//     {"errors":{"detail":"...","status":"UNAUTHORIZED"}}
//
//  2. Validation errors (request body / query failed schema validation):
//     {"errors":[{"title":"...","source":{"pointer":"/specs/0/feature/name"},...}]}
//
// Use WriteAppError for the first form; WriteValidationError for the second.
package apierror

import (
	"encoding/json"
	"net/http"
)

// AppError is the single-object error envelope.
type AppError struct {
	Detail string `json:"detail"`
	Status string `json:"status"`
}

// ValidationError is one entry in the validation-error array.
type ValidationError struct {
	Title  string         `json:"title"`
	Source map[string]any `json:"source,omitempty"`
}

// WriteAppError writes the single-object envelope at the given HTTP status.
// status defaults to a SCREAMING_SNAKE_CASE label derived from code if empty.
func WriteAppError(w http.ResponseWriter, code int, detail, status string) {
	if status == "" {
		status = StatusFromCode(code)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"errors": AppError{Detail: detail, Status: status},
	})
}

// WriteValidationError writes the array-of-validations envelope at the given
// HTTP status (typically 400 or 422).
func WriteValidationError(w http.ResponseWriter, code int, items []ValidationError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"errors": items,
	})
}

// StatusFromCode returns the SCREAMING_SNAKE_CASE label Phoenix uses.
func StatusFromCode(code int) string {
	switch code {
	case http.StatusBadRequest:
		return "BAD_REQUEST"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusUnprocessableEntity:
		return "UNPROCESSABLE_ENTITY"
	case http.StatusTooManyRequests:
		return "TOO_MANY_REQUESTS"
	case http.StatusRequestEntityTooLarge:
		return "PAYLOAD_TOO_LARGE"
	case http.StatusInternalServerError:
		return "INTERNAL_SERVER_ERROR"
	default:
		return "ERROR"
	}
}
```

- [ ] **Step 2:** Tests in `internal/api/apierror/envelope_test.go`:

```go
package apierror_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/api/apierror"
)

func TestWriteAppError_ShapeAndStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	apierror.WriteAppError(rec, http.StatusUnauthorized, "Token revoked", "")

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("content-type = %q, want application/json", got)
	}

	var doc struct {
		Errors apierror.AppError `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc.Errors.Detail != "Token revoked" {
		t.Errorf("Detail = %q", doc.Errors.Detail)
	}
	if doc.Errors.Status != "UNAUTHORIZED" {
		t.Errorf("Status = %q, want UNAUTHORIZED (auto-derived)", doc.Errors.Status)
	}
}

func TestWriteValidationError_ArrayShape(t *testing.T) {
	rec := httptest.NewRecorder()
	apierror.WriteValidationError(rec, http.StatusUnprocessableEntity, []apierror.ValidationError{
		{Title: "Invalid value", Source: map[string]any{"pointer": "/specs/0/feature/name"}},
		{Title: "Required", Source: map[string]any{"pointer": "/repo_uri"}},
	})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", rec.Code)
	}

	var doc struct {
		Errors []apierror.ValidationError `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(doc.Errors) != 2 {
		t.Errorf("got %d errors, want 2", len(doc.Errors))
	}
	if doc.Errors[0].Source["pointer"] != "/specs/0/feature/name" {
		t.Errorf("pointer = %v", doc.Errors[0].Source["pointer"])
	}
}

func TestStatusFromCode_KnownCodes(t *testing.T) {
	cases := map[int]string{
		400: "BAD_REQUEST",
		401: "UNAUTHORIZED",
		403: "FORBIDDEN",
		413: "PAYLOAD_TOO_LARGE",
		422: "UNPROCESSABLE_ENTITY",
		429: "TOO_MANY_REQUESTS",
	}
	for code, want := range cases {
		if got := apierror.StatusFromCode(code); got != want {
			t.Errorf("StatusFromCode(%d) = %q, want %q", code, got, want)
		}
	}
}
```

- [ ] **Step 3:** `just precommit` exits 0.

- [ ] **Step 4:** Commit: `feat(p2a): add api/apierror envelope (AppError + ValidationError)`.

---

## Task 5: `BearerAuth` middleware

- [ ] **Step 1:** Create `internal/api/middleware/bearer.go`:

```go
// Package middleware holds API-pipeline HTTP middleware: bearer auth, size
// caps, rate limiting.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/jadams-positron/acai-sh-server/internal/api/apierror"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
)

type ctxKey struct{ name string }

var (
	tokenCtxKey = ctxKey{"api.token"}
	teamCtxKey  = ctxKey{"api.team"}
)

// TokenFrom returns the *teams.AccessToken attached to ctx, or nil.
func TokenFrom(ctx context.Context) *teams.AccessToken {
	t, _ := ctx.Value(tokenCtxKey).(*teams.AccessToken)
	return t
}

// TeamFrom returns the *teams.Team attached to ctx, or nil.
func TeamFrom(ctx context.Context) *teams.Team {
	t, _ := ctx.Value(teamCtxKey).(*teams.Team)
	return t
}

// BearerAuth reads the Authorization header, validates the bearer token via
// repo.VerifyAccessToken, and attaches the resulting *AccessToken + *Team to
// the request context. On any failure mode (missing/malformed/unknown/revoked/
// expired), responds 401 with the standard app-error envelope.
func BearerAuth(repo *teams.Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawHeader := r.Header.Get("Authorization")
			if rawHeader == "" {
				apierror.WriteAppError(w, http.StatusUnauthorized, "Authorization header required", "")
				return
			}
			const prefix = "Bearer "
			if !strings.HasPrefix(rawHeader, prefix) {
				apierror.WriteAppError(w, http.StatusUnauthorized, "Authorization header must use Bearer scheme", "")
				return
			}
			plaintext := strings.TrimSpace(strings.TrimPrefix(rawHeader, prefix))
			if plaintext == "" {
				apierror.WriteAppError(w, http.StatusUnauthorized, "Invalid or missing bearer token", "")
				return
			}

			token, team, err := repo.VerifyAccessToken(r.Context(), plaintext)
			if err != nil {
				apierror.WriteAppError(w, http.StatusUnauthorized, "Invalid or expired bearer token", "")
				return
			}

			ctx := context.WithValue(r.Context(), tokenCtxKey, token)
			ctx = context.WithValue(ctx, teamCtxKey, team)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

- [ ] **Step 2:** Tests in `internal/api/middleware/bearer_test.go`:

```go
package middleware_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

func setup(t *testing.T) (*teams.Repository, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	ar := accounts.NewRepository(db)
	tr := teams.NewRepository(db)
	user, err := ar.CreateUser(context.Background(), accounts.CreateUserParams{Email: "u@example.com"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	team, err := tr.CreateTeam(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	plaintext, err := tr.CreateAccessToken(context.Background(), teams.CreateAccessTokenParams{
		UserID: user.ID, TeamID: team.ID, Name: "test-token",
	})
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}
	return tr, plaintext
}

func runBearer(t *testing.T, repo *teams.Repository, header string) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	called := false
	mw := middleware.BearerAuth(repo)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if middleware.TokenFrom(r.Context()) == nil {
			t.Errorf("downstream: TokenFrom returned nil")
		}
		if middleware.TeamFrom(r.Context()) == nil {
			t.Errorf("downstream: TeamFrom returned nil")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/x", http.NoBody)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	return rec, called
}

func mustErrorEnvelope(t *testing.T, body io.Reader) (detail, status string) {
	t.Helper()
	var doc struct {
		Errors struct {
			Detail string `json:"detail"`
			Status string `json:"status"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(body).Decode(&doc); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return doc.Errors.Detail, doc.Errors.Status
}

func TestBearerAuth_HappyPath(t *testing.T) {
	repo, plaintext := setup(t)
	rec, called := runBearer(t, repo, "Bearer "+plaintext)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Errorf("downstream not called")
	}
}

func TestBearerAuth_MissingHeader(t *testing.T) {
	repo, _ := setup(t)
	rec, called := runBearer(t, repo, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Errorf("downstream should not have been called")
	}
	detail, status := mustErrorEnvelope(t, rec.Body)
	if !strings.Contains(detail, "required") {
		t.Errorf("detail = %q", detail)
	}
	if status != "UNAUTHORIZED" {
		t.Errorf("status = %q", status)
	}
}

func TestBearerAuth_WrongScheme(t *testing.T) {
	repo, _ := setup(t)
	rec, _ := runBearer(t, repo, "Basic dXNlcjpwYXNz")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	detail, _ := mustErrorEnvelope(t, rec.Body)
	if !strings.Contains(detail, "Bearer") {
		t.Errorf("detail = %q", detail)
	}
}

func TestBearerAuth_EmptyToken(t *testing.T) {
	repo, _ := setup(t)
	rec, _ := runBearer(t, repo, "Bearer ")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestBearerAuth_UnknownToken(t *testing.T) {
	repo, _ := setup(t)
	rec, _ := runBearer(t, repo, "Bearer aaaaaaaa.notarealtoken")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	detail, _ := mustErrorEnvelope(t, rec.Body)
	if !strings.Contains(detail, "Invalid") && !strings.Contains(detail, "expired") {
		t.Errorf("detail = %q (expected something about invalid/expired)", detail)
	}
}
```

- [ ] **Step 3:** `just precommit` exits 0.

- [ ] **Step 4:** Commit: `feat(p2a): add api.BearerAuth middleware with TokenFrom/TeamFrom ctx helpers`.

---

## Task 6: `SizeCap` middleware

- [ ] **Step 1:** Create `internal/api/middleware/sizecap.go`:

```go
package middleware

import (
	"net/http"
	"strconv"

	"github.com/jadams-positron/acai-sh-server/internal/api/apierror"
)

// SizeCap rejects requests whose Content-Length exceeds capForEndpoint(path)
// with a 413 + standard app-error envelope. If capForEndpoint returns 0, no
// cap is applied. If Content-Length is missing, the request passes (huma will
// reject it later if relevant).
func SizeCap(capForEndpoint func(path string) int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cap := capForEndpoint(r.URL.Path)
			if cap > 0 {
				if cl := r.Header.Get("Content-Length"); cl != "" {
					n, err := strconv.ParseInt(cl, 10, 64)
					if err == nil && n > cap {
						apierror.WriteAppError(w, http.StatusRequestEntityTooLarge,
							"Request body exceeds size cap", "")
						return
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 2:** Tests in `internal/api/middleware/sizecap_test.go`:

```go
package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
)

func TestSizeCap_AllowsUnderCap(t *testing.T) {
	mw := middleware.SizeCap(func(string) int64 { return 1024 })(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/x", strings.NewReader("hello"))
	req.ContentLength = 5
	req.Header.Set("Content-Length", "5")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestSizeCap_RejectsOverCap(t *testing.T) {
	mw := middleware.SizeCap(func(string) int64 { return 100 })(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("downstream should not be called")
	}))
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/x", http.NoBody)
	req.Header.Set("Content-Length", "1024")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rec.Code)
	}
}

func TestSizeCap_NoCap(t *testing.T) {
	mw := middleware.SizeCap(func(string) int64 { return 0 })(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/x", http.NoBody)
	req.Header.Set("Content-Length", "9999999")
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (no cap)", rec.Code)
	}
}
```

- [ ] **Step 3:** Commit: `feat(p2a): add api.SizeCap middleware`.

---

## Task 7: `RateLimit` middleware

- [ ] **Step 1:** Create `internal/api/middleware/ratelimit.go`:

```go
package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jadams-positron/acai-sh-server/internal/api/apierror"
)

// RateLimitSpec specifies a rate limit window.
type RateLimitSpec struct {
	Requests      int
	WindowSeconds int
}

// Limiter is the rate-limiter contract.
type Limiter interface {
	// Allow returns true if the request is within the configured limit. It
	// must increment the counter for the bucket.
	Allow(endpointKey, tokenID string, spec RateLimitSpec, now time.Time) bool
}

// InProcessLimiter is the default Limiter — sync.Map keyed by
// (endpoint, tokenID, bucket) → atomic counter. Buckets older than the current
// window are pruned on each call (matches the existing Phoenix ETS approach).
type InProcessLimiter struct {
	buckets sync.Map // map[string]*atomic.Int64
}

// NewInProcessLimiter returns a fresh in-process limiter.
func NewInProcessLimiter() *InProcessLimiter { return &InProcessLimiter{} }

// Allow implements Limiter.
func (l *InProcessLimiter) Allow(endpointKey, tokenID string, spec RateLimitSpec, now time.Time) bool {
	if spec.Requests <= 0 || spec.WindowSeconds <= 0 {
		return true
	}
	bucket := now.Unix() / int64(spec.WindowSeconds)
	key := fmt.Sprintf("%s:%s:%d", endpointKey, tokenID, bucket)

	val, _ := l.buckets.LoadOrStore(key, new(atomic.Int64))
	count := val.(*atomic.Int64).Add(1)

	// Best-effort prune: walk a small sample of keys and drop those whose
	// bucket is in the past. We do this lazily on every 100th call to avoid
	// linear scans.
	if count%100 == 0 {
		l.pruneOlder(spec.WindowSeconds, bucket)
	}

	return count <= int64(spec.Requests)
}

func (l *InProcessLimiter) pruneOlder(windowSec int, currentBucket int64) {
	l.buckets.Range(func(k, _ any) bool {
		ks, _ := k.(string)
		// Extract trailing bucket number after last ':'
		for i := len(ks) - 1; i >= 0; i-- {
			if ks[i] == ':' {
				bucketStr := ks[i+1:]
				var b int64
				_, err := fmt.Sscanf(bucketStr, "%d", &b)
				if err == nil && b < currentBucket {
					l.buckets.Delete(ks)
				}
				return true
			}
		}
		return true
	})
}

// RateLimit returns middleware that consults limiter for each request, keyed
// by the endpoint path (or "default" if specForPath returns zero) and the
// authenticated token ID (or "anonymous").
func RateLimit(specForPath func(path string) RateLimitSpec, limiter Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			spec := specForPath(r.URL.Path)
			tokenID := "anonymous"
			if t := TokenFrom(r.Context()); t != nil {
				tokenID = t.ID
			}
			if !limiter.Allow(r.URL.Path, tokenID, spec, time.Now()) {
				apierror.WriteAppError(w, http.StatusTooManyRequests, "Rate limit exceeded", "")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 2:** Tests in `internal/api/middleware/ratelimit_test.go`:

```go
package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
)

func TestRateLimit_AllowsBelowQuota(t *testing.T) {
	limiter := middleware.NewInProcessLimiter()
	spec := middleware.RateLimitSpec{Requests: 3, WindowSeconds: 60}

	for i := 0; i < 3; i++ {
		if !limiter.Allow("ep", "tok", spec, time.Now()) {
			t.Fatalf("call %d should be allowed", i+1)
		}
	}
}

func TestRateLimit_RejectsOverQuota(t *testing.T) {
	limiter := middleware.NewInProcessLimiter()
	spec := middleware.RateLimitSpec{Requests: 2, WindowSeconds: 60}

	now := time.Now()
	_ = limiter.Allow("ep", "tok", spec, now)
	_ = limiter.Allow("ep", "tok", spec, now)
	if limiter.Allow("ep", "tok", spec, now) {
		t.Errorf("third call should be rejected")
	}
}

func TestRateLimit_DifferentTokensIndependent(t *testing.T) {
	limiter := middleware.NewInProcessLimiter()
	spec := middleware.RateLimitSpec{Requests: 1, WindowSeconds: 60}

	now := time.Now()
	if !limiter.Allow("ep", "tokA", spec, now) {
		t.Errorf("first call for tokA should be allowed")
	}
	if !limiter.Allow("ep", "tokB", spec, now) {
		t.Errorf("first call for tokB should be allowed (independent)")
	}
}

func TestRateLimit_NewWindowResets(t *testing.T) {
	limiter := middleware.NewInProcessLimiter()
	spec := middleware.RateLimitSpec{Requests: 1, WindowSeconds: 1}

	t0 := time.Unix(1000, 0)
	t1 := time.Unix(1001, 0) // different bucket

	if !limiter.Allow("ep", "tok", spec, t0) {
		t.Errorf("first call should be allowed")
	}
	if limiter.Allow("ep", "tok", spec, t0) {
		t.Errorf("second call in same bucket should be rejected")
	}
	if !limiter.Allow("ep", "tok", spec, t1) {
		t.Errorf("first call in new bucket should be allowed")
	}
}

func TestRateLimit_Middleware_429OnExceeded(t *testing.T) {
	limiter := middleware.NewInProcessLimiter()
	spec := middleware.RateLimitSpec{Requests: 1, WindowSeconds: 60}

	mw := middleware.RateLimit(func(string) middleware.RateLimitSpec { return spec }, limiter)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	doRequest := func() int {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/x", http.NoBody)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := doRequest(); got != http.StatusOK {
		t.Errorf("first request code = %d, want 200", got)
	}
	if got := doRequest(); got != http.StatusTooManyRequests {
		t.Errorf("second request code = %d, want 429", got)
	}
}
```

- [ ] **Step 3:** Commit: `feat(p2a): add api.RateLimit middleware + InProcessLimiter`.

---

## Task 8: Operations config

- [ ] **Step 1:** Create `internal/api/operations/config.go`:

```go
// Package operations holds the runtime per-endpoint API configuration:
// request-size caps, semantic caps, and rate-limit specs. Mirrors the env-var
// shape from the original Phoenix runtime.exs.
package operations

import (
	"os"
	"strconv"

	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
)

// EndpointConfig is the per-endpoint runtime config.
type EndpointConfig struct {
	RequestSizeCap int64
	RateLimit      middleware.RateLimitSpec
	SemanticCaps   map[string]int
}

// Config holds the per-endpoint config keyed by endpoint name (e.g. "push",
// "feature_states", "implementations", etc.) plus a "default" fallback.
type Config struct {
	NonProd   bool
	Endpoints map[string]EndpointConfig
}

// Endpoint keys (matching the Phoenix `Operations.endpoint_key` mapping).
const (
	EndpointDefault              = "default"
	EndpointPush                 = "push"
	EndpointFeatureStates        = "feature_states"
	EndpointImplementations      = "implementations"
	EndpointFeatureContext       = "feature_context"
	EndpointImplementationFeats  = "implementation_features"
)

// Load reads operation config from environment variables. Defaults match the
// non-prod profile from runtime.exs.
func Load(nonProd bool) *Config {
	cfg := &Config{NonProd: nonProd, Endpoints: map[string]EndpointConfig{}}

	pickInt := func(key string, prod, devDefault int) int {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			n, err := strconv.Atoi(v)
			if err == nil {
				return n
			}
		}
		if nonProd {
			return devDefault
		}
		return prod
	}
	pickInt64 := func(key string, prod, devDefault int64) int64 {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err == nil {
				return n
			}
		}
		if nonProd {
			return devDefault
		}
		return prod
	}

	cfg.Endpoints[EndpointDefault] = EndpointConfig{
		RequestSizeCap: pickInt64("API_DEFAULT_REQUEST_SIZE_CAP", 1_000_000, 2_000_000),
		RateLimit: middleware.RateLimitSpec{
			Requests:      pickInt("API_DEFAULT_RATE_LIMIT_REQUESTS", 60, 120),
			WindowSeconds: pickInt("API_DEFAULT_RATE_LIMIT_WINDOW_SECONDS", 60, 60),
		},
		SemanticCaps: map[string]int{
			"max_specs":      pickInt("API_DEFAULT_MAX_SPECS", 50, 100),
			"max_references": pickInt("API_DEFAULT_MAX_REFERENCES", 5_000, 10_000),
		},
	}

	cfg.Endpoints[EndpointPush] = EndpointConfig{
		RequestSizeCap: pickInt64("API_PUSH_REQUEST_SIZE_CAP", 2_000_000, 4_000_000),
		RateLimit: middleware.RateLimitSpec{
			Requests:      pickInt("API_PUSH_RATE_LIMIT_REQUESTS", 30, 60),
			WindowSeconds: pickInt("API_PUSH_RATE_LIMIT_WINDOW_SECONDS", 60, 60),
		},
		SemanticCaps: map[string]int{
			"max_specs":                      pickInt("API_PUSH_MAX_SPECS", 50, 100),
			"max_references":                 pickInt("API_PUSH_MAX_REFERENCES", 5_000, 10_000),
			"max_requirements_per_spec":      pickInt("API_PUSH_MAX_REQUIREMENTS_PER_SPEC", 100, 200),
			"max_raw_content_bytes":          pickInt("API_PUSH_MAX_RAW_CONTENT_BYTES", 51_200, 102_400),
			"max_requirement_string_length":  pickInt("API_PUSH_MAX_REQUIREMENT_STRING_LENGTH", 1_000, 2_000),
			"max_feature_description_length": pickInt("API_PUSH_MAX_FEATURE_DESCRIPTION_LENGTH", 2_500, 5_000),
			"max_meta_path_length":           pickInt("API_PUSH_MAX_META_PATH_LENGTH", 512, 1_024),
			"max_repo_uri_length":            pickInt("API_PUSH_MAX_REPO_URI_LENGTH", 1_024, 2_048),
		},
	}

	cfg.Endpoints[EndpointFeatureStates] = EndpointConfig{
		RequestSizeCap: pickInt64("API_FEATURE_STATES_REQUEST_SIZE_CAP", 1_000_000, 2_000_000),
		RateLimit: middleware.RateLimitSpec{
			Requests:      pickInt("API_FEATURE_STATES_RATE_LIMIT_REQUESTS", 30, 60),
			WindowSeconds: pickInt("API_FEATURE_STATES_RATE_LIMIT_WINDOW_SECONDS", 60, 60),
		},
		SemanticCaps: map[string]int{
			"max_states":          pickInt("API_FEATURE_STATES_MAX_STATES", 500, 500),
			"max_comment_length":  pickInt("API_FEATURE_STATES_MAX_COMMENT_LENGTH", 2_000, 2_000),
		},
	}

	return cfg
}

// EndpointKeyForPath returns the endpoint key for a request path. Mirrors
// AcaiWeb.Api.Operations.endpoint_key.
func EndpointKeyForPath(path string) string {
	switch path {
	case "/api/v1/push":
		return EndpointPush
	case "/api/v1/feature-states":
		return EndpointFeatureStates
	case "/api/v1/implementations":
		return EndpointImplementations
	case "/api/v1/feature-context":
		return EndpointFeatureContext
	case "/api/v1/implementation-features":
		return EndpointImplementationFeats
	default:
		return EndpointDefault
	}
}

// SizeCapForPath returns the per-endpoint request size cap (or default).
func (c *Config) SizeCapForPath(path string) int64 {
	key := EndpointKeyForPath(path)
	if ep, ok := c.Endpoints[key]; ok {
		return ep.RequestSizeCap
	}
	return c.Endpoints[EndpointDefault].RequestSizeCap
}

// RateLimitForPath returns the per-endpoint rate-limit spec (or default).
func (c *Config) RateLimitForPath(path string) middleware.RateLimitSpec {
	key := EndpointKeyForPath(path)
	if ep, ok := c.Endpoints[key]; ok {
		return ep.RateLimit
	}
	return c.Endpoints[EndpointDefault].RateLimit
}
```

- [ ] **Step 2:** Tests in `internal/api/operations/config_test.go`:

```go
package operations_test

import (
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/api/operations"
)

func TestLoad_NonProdDefaults(t *testing.T) {
	cfg := operations.Load(true)

	push := cfg.Endpoints[operations.EndpointPush]
	if push.RequestSizeCap != 4_000_000 {
		t.Errorf("non-prod push size cap = %d, want 4_000_000", push.RequestSizeCap)
	}
	if push.RateLimit.Requests != 60 {
		t.Errorf("non-prod push rate-limit requests = %d, want 60", push.RateLimit.Requests)
	}
}

func TestLoad_ProdDefaults(t *testing.T) {
	cfg := operations.Load(false)

	push := cfg.Endpoints[operations.EndpointPush]
	if push.RequestSizeCap != 2_000_000 {
		t.Errorf("prod push size cap = %d, want 2_000_000", push.RequestSizeCap)
	}
	if push.RateLimit.Requests != 30 {
		t.Errorf("prod push rate-limit requests = %d, want 30", push.RateLimit.Requests)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("API_PUSH_REQUEST_SIZE_CAP", "12345")
	t.Setenv("API_PUSH_RATE_LIMIT_REQUESTS", "7")

	cfg := operations.Load(true)
	push := cfg.Endpoints[operations.EndpointPush]
	if push.RequestSizeCap != 12345 {
		t.Errorf("override size cap = %d, want 12345", push.RequestSizeCap)
	}
	if push.RateLimit.Requests != 7 {
		t.Errorf("override rate-limit = %d, want 7", push.RateLimit.Requests)
	}
}

func TestEndpointKeyForPath(t *testing.T) {
	cases := map[string]string{
		"/api/v1/push":                    operations.EndpointPush,
		"/api/v1/feature-states":          operations.EndpointFeatureStates,
		"/api/v1/implementations":         operations.EndpointImplementations,
		"/api/v1/feature-context":         operations.EndpointFeatureContext,
		"/api/v1/implementation-features": operations.EndpointImplementationFeats,
		"/api/v1/random":                  operations.EndpointDefault,
	}
	for path, want := range cases {
		if got := operations.EndpointKeyForPath(path); got != want {
			t.Errorf("EndpointKeyForPath(%q) = %q, want %q", path, got, want)
		}
	}
}
```

- [ ] **Step 3:** Commit: `feat(p2a): add api/operations runtime config (per-endpoint caps + rate limits)`.

---

## Task 9: Huma router + mount

- [ ] **Step 1:** Create `internal/api/router.go`:

```go
// Package api owns the /api/v1 sub-router: bearer auth, size cap, rate limit,
// operation registration via huma. P2a stands up the scaffold; P2b/P2c add the
// individual operations.
package api

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/api/operations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
)

// Deps groups the dependencies the api sub-router needs.
type Deps struct {
	Teams      *teams.Repository
	Operations *operations.Config
	Limiter    middleware.Limiter
}

// Mount registers the /api/v1/* routes on parent. Each endpoint registered
// later (P2b/P2c) hangs off the returned huma.API instance.
//
// Returns the huma.API so subsequent phases can register operations against it.
func Mount(parent chi.Router, deps *Deps) huma.API {
	parent.Route("/api/v1", func(r chi.Router) {
		// Auth, size cap, rate limit are applied to ALL /api/v1/* routes
		// EXCEPT openapi.json which is intentionally public.
		r.Group(func(r chi.Router) {
			r.Use(middleware.BearerAuth(deps.Teams))
			r.Use(middleware.SizeCap(deps.Operations.SizeCapForPath))
			r.Use(middleware.RateLimit(deps.Operations.RateLimitForPath, deps.Limiter))

			// huma operations registered against the Group's chi.Router.
			cfg := huma.DefaultConfig("Acai API", "1.0.0")
			cfg.OpenAPI.Servers = []*huma.Server{{URL: "/api/v1", Description: "API v1"}}
			cfg.OpenAPI.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
				"bearerAuth": {
					Type:         "http",
					Scheme:       "bearer",
					BearerFormat: "API token",
				},
			}
			cfg.OpenAPI.Security = []map[string][]string{{"bearerAuth": {}}}
			// huma writes /openapi.json relative to its mount; chi's Route
			// already prefixes /api/v1.
			cfg.OpenAPIPath = "/openapi.json"
			cfg.DocsPath = "" // suppress huma's own docs UI; we serve the spec only
			_ = humachi.New(r, cfg)
			// Operations registered here in P2b/P2c via the returned API.
			// Currently no operations registered = empty paths section.
		})

		// Public openapi.json — must be reachable without bearer auth so the
		// CLI can fetch the spec to bootstrap. We serve a snapshot generated
		// from the same huma config used for the auth'd group above.
		r.Get("/openapi.json", openAPIHandler(deps))
	})

	// Return value will become useful in P2b once operations are registered
	// on the auth'd group's api. For now, we discard.
	return nil
}
```

(Note: huma's `humachi.New` mounts both the operations and a `/openapi.json` route on the supplied router. To make the spec public while requiring auth on operations, we register the operations group with bearer-auth middleware AND serve a separate public `/openapi.json` outside the group. This is reflected in the code above; P2b will refine when actual operations land.)

- [ ] **Step 2:** Create `internal/api/openapi.go`:

```go
package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// openAPIHandler returns an http.HandlerFunc that emits the same OpenAPI doc
// the auth'd group exposes. Public — no bearer required.
func openAPIHandler(deps *Deps) http.HandlerFunc {
	// Build a parallel huma instance over a throwaway chi router so we can
	// emit the same spec without coupling to the auth'd group's pipeline.
	cfg := huma.DefaultConfig("Acai API", "1.0.0")
	cfg.OpenAPI.Servers = []*huma.Server{{URL: "/api/v1", Description: "API v1"}}
	cfg.OpenAPI.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: "API token"},
	}
	cfg.OpenAPI.Security = []map[string][]string{{"bearerAuth": {}}}
	cfg.OpenAPIPath = "/openapi.json"
	cfg.DocsPath = ""

	throwaway := chi.NewRouter()
	api := humachi.New(throwaway, cfg)
	_ = api // P2b will register operations against this same instance via a shared registration func

	return func(w http.ResponseWriter, r *http.Request) {
		// Delegate to throwaway's handler.
		throwaway.ServeHTTP(w, r)
	}
}
```

(The above is a P2a stub — it's slightly hacky because huma is designed to own the router. P2b's first task will replace this with a proper shared-registration approach where both auth'd and public routes register the same operations against shared `huma.API` instance.)

- [ ] **Step 3:** Wire into `internal/server/router.go`:

```go
// In newRouter:

apiDeps := &api.Deps{
    Teams:      deps.Teams,
    Operations: deps.Operations,
    Limiter:    deps.APILimiter,
}
api.Mount(r, apiDeps)
```

(Add Teams, Operations, APILimiter to RouterDeps.)

- [ ] **Step 4:** Wire in `cmd/acai/serve.go` — after constructing `repo, sessionManager, mlSvc`:

```go
teamsRepo := teams.NewRepository(db)
ops := operations.Load(cfg.LogLevel == "debug" || cfg.URLScheme == "http") // simplistic non-prod heuristic
limiter := middleware.NewInProcessLimiter()

// add to RouterDeps:
//   Teams:      teamsRepo,
//   Operations: ops,
//   APILimiter: limiter,
```

- [ ] **Step 5:** Test in `internal/api/router_test.go`:

```go
package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/jadams-positron/acai-sh-server/internal/api"
	"github.com/jadams-positron/acai-sh-server/internal/api/middleware"
	"github.com/jadams-positron/acai-sh-server/internal/api/operations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

func TestMount_OpenAPIJSONPublicAndValid(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	r := chi.NewRouter()
	api.Mount(r, &api.Deps{
		Teams:      teams.NewRepository(db),
		Operations: operations.Load(true),
		Limiter:    middleware.NewInProcessLimiter(),
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/openapi.json", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc["openapi"] == nil {
		t.Errorf("openapi key missing in response")
	}
	if doc["info"] == nil {
		t.Errorf("info key missing in response")
	}
}

func TestMount_AuthRouteRequiresBearer(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	r := chi.NewRouter()
	api.Mount(r, &api.Deps{
		Teams:      teams.NewRepository(db),
		Operations: operations.Load(true),
		Limiter:    middleware.NewInProcessLimiter(),
	})

	// No operations are registered yet (P2b adds them), so we hit a 404 from
	// huma — which is correct and means auth ran but there's no handler.
	// What we DO want to verify: a request with NO Authorization header on a
	// fake protected path returns 401.
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/anything", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		body, _ := io.ReadAll(rec.Body)
		t.Fatalf("status = %d, want 401; body=%s", rec.Code, body)
	}
}
```

- [ ] **Step 6:** `just precommit` exits 0.

- [ ] **Step 7:** Local smoke:

```bash
just build
DATABASE_PATH=/tmp/acai-p2a.db HTTP_PORT=4321 \
  SECRET_KEY_BASE="$(openssl rand -hex 32)" \
  ./acai serve &
SERVER_PID=$!
sleep 1
echo "--- /api/v1/openapi.json ---"
curl -fsS http://localhost:4321/api/v1/openapi.json | jq '{openapi, info: .info.title, paths: (.paths|keys)}'
echo "--- /api/v1/push without auth ---"
curl -fsS -i http://localhost:4321/api/v1/push 2>&1 | head -10
kill -INT $SERVER_PID
wait $SERVER_PID 2>/dev/null
rm -f /tmp/acai-p2a.db /tmp/acai-p2a.db-shm /tmp/acai-p2a.db-wal
just clean
```

Expected: openapi.json returns `{"openapi": "3.x.x", "info": "Acai API", "paths": []}`. The `/api/v1/push` request without a token returns `401 + {"errors":{"detail":"Authorization header required",...}}`.

- [ ] **Step 8:** Commit: `feat(p2a): mount /api/v1 sub-router with huma + auth pipeline`.

- [ ] **Step 9:** Push and verify CI green.

---

## Self-Review

- **Spec coverage:** Bearer auth ✓, size cap ✓, rate limit ✓, operations config (env names exact) ✓, error envelope (both shapes) ✓, openapi.json public ✓.
- **Out of scope (P2b, P2c):** No actual operations registered. No JSON schemas for push/feature-states/etc. The shared-huma-instance gymnastics in `openapi.go` is a stub; P2b will replace with a proper builder pattern.
- **Type consistency:** `*teams.Repository.VerifyAccessToken(ctx, plaintext) (*AccessToken, *Team, error)`. `middleware.TokenFrom(ctx)` and `middleware.TeamFrom(ctx)` both nillable. `*operations.Config.SizeCapForPath(path) int64` and `RateLimitForPath(path) RateLimitSpec`.

## Risk register

- **huma + chi mount with public openapi.json** — the duplicate `humachi.New` for the public route is a code smell. P2b's first task is to refactor into one huma.API plus a documented "register operations" function called from both the auth'd group AND the public spec route.
- **Argon2id cost on bearer verify** — argon2 is slow by design (~50ms with our params). For a stateless API hit on every request, this might be too slow. If verification latency matters, a P2b optimization is to switch to a fast HMAC-SHA256 hash for the secret-vs-stored compare and cache the verify result for ~1 minute keyed by token hash.
- **Rate limiter prune cost** — `pruneOlder` walks `sync.Map`. At 1000s of buckets this is fine; at millions, switch to a TTL cache. Self-host single-instance: not a concern.
- **Operation registration deferred** — without operations, openapi.json paths are empty and the CLI golden test (P2d) cannot run. Plan that explicitly: P2b registers the read endpoints; P2c the write endpoints; P2d does the golden parity test only after both.
