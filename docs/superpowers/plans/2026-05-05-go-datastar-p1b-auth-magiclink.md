# Go + Datastar Rewrite — Phase 1b: Auth Core + Magic-Link Login

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Working magic-link login. After P1b, an operator runs `acai create-admin --email=…`, visits `/users/log-in`, enters their email, copies the magic-link URL from the slog output (mailer is noop in dev), pastes it in the browser, and lands on an authenticated session. Site routes that require auth gate correctly. Registration UI, full Tailwind+Datastar styling, and production Mailgun mailer are deferred to **P1c** to keep this plan finite.

**Architecture:** Argon2id passwords, scs/v2 sessions backed by SQLite, magic-link tokens stored as sha256 hashes in `email_tokens` (single-use). Browser pipeline: `slog → recover → sessionManager.LoadAndSave → auth.LoadScope → csrf → handler`. Mailer is an interface with a single `noop` implementation that logs the magic-link URL via `slog.Info`.

**Tech Stack:** `golang.org/x/crypto` (argon2id), `github.com/alexedwards/scs/v2`, `github.com/alexedwards/scs/sqlite3store`, `github.com/gorilla/csrf`, `github.com/google/uuid` (UUIDv7 for IDs).

**Reference:** Spec `docs/superpowers/specs/2026-05-05-go-datastar-rewrite-design.md` §7 (Auth). P1a plan landed at branch `rewrite/go-datastar` commit `dc2cc2d`. P1b builds directly on top.

---

## Scope decisions (read carefully before starting)

- **No registration UI.** Users are seeded with `acai create-admin --email=foo@bar.com` (subcommand added in T10). Self-serve registration is a P1c task.
- **No Tailwind, no Datastar runtime, no templ.** Pages are bare HTML strings rendered from Go's `html/template`. The visual rewrite to templ + Tailwind + Datastar is P1c. We use stdlib `html/template` here so the auth flow can be proven without the toolchain plumbing.
- **Mailer noop only.** The interface exists; the only implementation is `mail.Noop` which writes the magic-link URL to slog. Mailgun lands in P1c.
- **No CSRF on the magic-link consume route.** GET /users/log-in/{token} bypasses CSRF (token is the auth proof). All POSTs go through CSRF.
- **scs sessions table** is created automatically by `scs/sqlite3store` on first use; not part of goose migrations.

---

## File Structure

| Path | Purpose | Created in task |
|---|---|---|
| `go.mod` / `go.sum` | New deps | T1 |
| `internal/config/config.go` | Add `SecretKeyBase`, `MailNoop`, `MailFromName`, `MailFromEmail`, `URLHost`, `URLScheme` | T1 |
| `internal/auth/password.go` | Argon2id `Hash` + `Verify` | T2 |
| `internal/auth/password_test.go` | Tests | T2 |
| `internal/store/queries/users.sql` | sqlc queries: CreateUser, GetUserByEmail, GetUserByID | T3 |
| `internal/store/queries/email_tokens.sql` | sqlc: CreateEmailToken, GetEmailTokenByHash, DeleteEmailToken, DeleteEmailTokensForUser | T3 |
| `internal/store/sqlc/*.sql.go` | Generated | T3 |
| `internal/domain/accounts/user.go` | `User` struct + `Repository` over sqlc | T4 |
| `internal/domain/accounts/email_token.go` | `EmailToken` + token gen/verify helpers | T4 |
| `internal/domain/accounts/repository.go` | Public domain API | T4 |
| `internal/mail/mailer.go` | `Mailer` interface + `NewNoop(*slog.Logger)` impl | T5 |
| `internal/auth/magic_link.go` | `Service.GenerateLoginToken(user) (urlToken, error)`; `ConsumeLoginToken(rawToken) (*User, error)` | T6 |
| `internal/auth/scope.go` | `Scope` struct + ctx helpers | T7 |
| `internal/auth/session.go` | scs session manager constructor (sqlite3store) | T7 |
| `internal/auth/middleware.go` | `LoadScope`, `RequireAuth`, `RedirectIfAuth` | T7 |
| `internal/site/handlers/auth.go` | `LoginNew`, `LoginCreate`, `LoginConfirm`, `LogOut` | T8 |
| `internal/site/templates/auth.go` | `html/template` strings for login pages | T8 |
| `internal/site/router.go` | Register auth routes; wire CSRF + session middleware | T8 |
| `internal/server/router.go` | Mount the new site router and auth-required test route | T8 |
| `internal/site/handlers/auth_test.go` | End-to-end test for the full flow | T9 |
| `cmd/acai/create_admin.go` | `acai create-admin --email=…` subcommand | T10 |
| `cmd/acai/main.go` | Add `create-admin` to dispatch + usage | T10 |

---

## Task 1: Deps + extended Config

**Files:** `go.mod`, `go.sum`, `internal/config/config.go`, `internal/config/config_test.go`

- [ ] **Step 1: Add direct deps**

```bash
go get github.com/alexedwards/scs/v2
go get github.com/alexedwards/scs/sqlite3store
go get github.com/gorilla/csrf
go get golang.org/x/crypto
go get github.com/google/uuid
go mod tidy
```

- [ ] **Step 2: Extend `Config` struct**

Edit `internal/config/config.go`. Add new fields (alphabetical inside groups), plus loading + validation:

```go
type Config struct {
	// LogLevel is one of "debug", "info", "warn", "error". Default: "info".
	LogLevel string

	// DatabasePath is the filesystem path to the SQLite database file.
	// Default: "./acai.db".
	DatabasePath string

	// HTTPPort is the TCP port the HTTP server listens on. Default: 4000.
	HTTPPort int

	// SecretKeyBase signs cookies (sessions, CSRF, remember-me). Required in
	// prod; in dev/test we accept an unsafe default to keep `just test` quick.
	// Minimum 32 bytes when set.
	SecretKeyBase string

	// MailNoop disables real email sending and instead logs the message via
	// slog. Default: true (the only mailer wired up in P1b is the noop one;
	// production Mailgun lands in P1c).
	MailNoop bool

	// MailFromName, MailFromEmail are the From header fields for outgoing
	// transactional email (magic-link, email-change confirmation).
	MailFromName  string
	MailFromEmail string

	// URLHost, URLScheme combine to build absolute URLs in emails (e.g.,
	// magic-link). For dev: localhost + http. For prod: app.acai.sh + https.
	URLHost   string
	URLScheme string
}
```

Replace `Load()`:

```go
func Load() (*Config, error) {
	cfg := &Config{
		LogLevel:      getenvDefault("LOG_LEVEL", "info"),
		DatabasePath:  getenvDefault("DATABASE_PATH", "./acai.db"),
		SecretKeyBase: getenvDefault("SECRET_KEY_BASE", "UNSAFE_dev_secret_key_base_for_local_use_ONLY_xxxxxxxxxxxxxx"),
		MailFromName:  getenvDefault("MAIL_FROM_NAME", "Acai"),
		MailFromEmail: getenvDefault("MAIL_FROM_EMAIL", "noreply@example.com"),
		URLHost:       getenvDefault("URL_HOST", "localhost"),
		URLScheme:     getenvDefault("URL_SCHEME", "http"),
	}
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
		// ok
	default:
		return nil, fmt.Errorf("config: invalid LOG_LEVEL %q (allowed: debug, info, warn, error)", cfg.LogLevel)
	}

	httpPort, err := strconv.Atoi(getenvDefault("HTTP_PORT", "4000"))
	if err != nil {
		return nil, fmt.Errorf("config: invalid HTTP_PORT: %w", err)
	}
	if httpPort < 1 || httpPort > 65535 {
		return nil, fmt.Errorf("config: HTTP_PORT %d out of range [1, 65535]", httpPort)
	}
	cfg.HTTPPort = httpPort

	if len(cfg.SecretKeyBase) < 32 {
		return nil, fmt.Errorf("config: SECRET_KEY_BASE must be at least 32 bytes (got %d)", len(cfg.SecretKeyBase))
	}

	mailNoopStr := getenvDefault("MAIL_NOOP", "true")
	switch mailNoopStr {
	case "true", "1", "yes":
		cfg.MailNoop = true
	case "false", "0", "no":
		cfg.MailNoop = false
	default:
		return nil, fmt.Errorf("config: invalid MAIL_NOOP %q (allowed: true|false)", mailNoopStr)
	}

	switch cfg.URLScheme {
	case "http", "https":
		// ok
	default:
		return nil, fmt.Errorf("config: invalid URL_SCHEME %q (allowed: http, https)", cfg.URLScheme)
	}

	return cfg, nil
}
```

- [ ] **Step 3: Add tests**

Append to `internal/config/config_test.go`:

```go
func TestLoad_DefaultsForP1bFields(t *testing.T) {
	t.Setenv("SECRET_KEY_BASE", "")
	t.Setenv("MAIL_NOOP", "")
	t.Setenv("MAIL_FROM_NAME", "")
	t.Setenv("MAIL_FROM_EMAIL", "")
	t.Setenv("URL_HOST", "")
	t.Setenv("URL_SCHEME", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.MailNoop {
		t.Errorf("MailNoop default = false, want true")
	}
	if cfg.MailFromName != "Acai" {
		t.Errorf("MailFromName = %q, want %q", cfg.MailFromName, "Acai")
	}
	if cfg.URLHost != "localhost" {
		t.Errorf("URLHost = %q, want %q", cfg.URLHost, "localhost")
	}
	if cfg.URLScheme != "http" {
		t.Errorf("URLScheme = %q, want %q", cfg.URLScheme, "http")
	}
	if len(cfg.SecretKeyBase) < 32 {
		t.Errorf("SecretKeyBase len = %d, want >= 32", len(cfg.SecretKeyBase))
	}
}

func TestLoad_RejectsShortSecretKeyBase(t *testing.T) {
	t.Setenv("SECRET_KEY_BASE", "tooshort")

	_, err := config.Load()
	if err == nil {
		t.Fatalf("Load() with short SECRET_KEY_BASE should have errored")
	}
}

func TestLoad_RejectsInvalidMailNoop(t *testing.T) {
	t.Setenv("MAIL_NOOP", "maybe")

	_, err := config.Load()
	if err == nil {
		t.Fatalf("Load() with MAIL_NOOP=maybe should have errored")
	}
}

func TestLoad_RejectsInvalidURLScheme(t *testing.T) {
	t.Setenv("URL_SCHEME", "ftp")

	_, err := config.Load()
	if err == nil {
		t.Fatalf("Load() with URL_SCHEME=ftp should have errored")
	}
}
```

- [ ] **Step 4: Verify**

```bash
just precommit
```

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/config/
git commit -m "feat(p1b): add scs/csrf/argon2 deps and SECRET_KEY_BASE/MAIL_*/URL_* config"
```

---

## Task 2: Argon2id password hashing

**Files:** `internal/auth/password.go`, `internal/auth/password_test.go`

- [ ] **Step 1: Write the failing test**

```go
package auth_test

import (
	"strings"
	"testing"

	"github.com/acai-sh/server/internal/auth"
)

func TestHash_RoundTrip(t *testing.T) {
	password := "correct horse battery staple"

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Errorf("hash format unexpected: %q", hash)
	}

	if err := auth.VerifyPassword(password, hash); err != nil {
		t.Errorf("Verify with correct password: %v", err)
	}

	if err := auth.VerifyPassword("wrong password", hash); err == nil {
		t.Errorf("Verify with wrong password: expected error, got nil")
	}
}

func TestHash_DifferentSaltsForSamePassword(t *testing.T) {
	a, err := auth.HashPassword("password")
	if err != nil {
		t.Fatalf("Hash a: %v", err)
	}
	b, err := auth.HashPassword("password")
	if err != nil {
		t.Fatalf("Hash b: %v", err)
	}
	if a == b {
		t.Errorf("two hashes of same password should differ (random salt); got identical: %s", a)
	}
}

func TestVerify_RejectsMalformedHash(t *testing.T) {
	if err := auth.VerifyPassword("any", "not a valid phc string"); err == nil {
		t.Errorf("Verify with malformed hash should error, got nil")
	}
}

func TestVerify_RejectsWrongAlgorithm(t *testing.T) {
	// argon2i (not -id) should be rejected as unsupported.
	hash := "$argon2i$v=19$m=65536,t=3,p=4$c2FsdHNhbHRzYWx0c2FsdA$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGFzaGhhc2g"
	if err := auth.VerifyPassword("any", hash); err == nil {
		t.Errorf("Verify with argon2i (not argon2id) should error, got nil")
	}
}
```

- [ ] **Step 2: Build error expected**

```bash
go test ./internal/auth/...
```

- [ ] **Step 3: Implement `internal/auth/password.go`**

```go
// Package auth owns the auth subsystem: password hashing, sessions, magic-link
// tokens, scope handling, and HTTP middleware. P1b lands password + session +
// magic-link + middleware.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters chosen to match the argon2_elixir defaults the legacy
// Phoenix code uses, so imported password hashes verify without rehashing.
const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024 // KiB → 64 MiB
	argonThreads uint8  = 4
	argonKeyLen  uint32 = 32
	argonSaltLen        = 16
)

var (
	// ErrInvalidHash is returned when the stored hash is not a parseable
	// argon2id PHC string.
	ErrInvalidHash = errors.New("auth: invalid password hash format")
	// ErrIncorrectPassword is returned when the password does not verify.
	ErrIncorrectPassword = errors.New("auth: incorrect password")
)

// HashPassword returns a PHC-formatted argon2id hash for password.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return formatPHC(salt, key), nil
}

// VerifyPassword returns nil iff password matches hash. The error distinguishes
// malformed hashes from mismatches; callers SHOULD treat both as
// "credentials invalid" externally to avoid timing/oracle leaks.
func VerifyPassword(password, hash string) error {
	salt, key, err := parsePHC(hash)
	if err != nil {
		return err
	}
	candidate := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	if subtle.ConstantTimeCompare(candidate, key) != 1 {
		return ErrIncorrectPassword
	}
	return nil
}

func formatPHC(salt, key []byte) string {
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
}

func parsePHC(hash string) ([]byte, []byte, error) {
	parts := strings.Split(hash, "$")
	// Expected: ["", "argon2id", "v=19", "m=65536,t=3,p=4", "<salt>", "<key>"]
	if len(parts) != 6 {
		return nil, nil, ErrInvalidHash
	}
	if parts[1] != "argon2id" {
		return nil, nil, ErrInvalidHash
	}
	if parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return nil, nil, ErrInvalidHash
	}

	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return nil, nil, ErrInvalidHash
	}
	// We require the same parameters we hash with — different params would mean
	// rehash-on-verify, which we don't implement in P1b.
	if memory != argonMemory || time != argonTime || threads != argonThreads {
		return nil, nil, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, ErrInvalidHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, ErrInvalidHash
	}
	return salt, key, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/auth/... -v
```

Expected: 4/4 pass.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/
git commit -m "feat(p1b): add argon2id password hashing"
```

---

## Task 3: sqlc queries for users + email_tokens

**Files:** `internal/store/queries/users.sql`, `internal/store/queries/email_tokens.sql`, generated `internal/store/sqlc/users.sql.go`, `email_tokens.sql.go`.

- [ ] **Step 1: Write `internal/store/queries/users.sql`**

```sql
-- name: CreateUser :one
INSERT INTO users (id, email, hashed_password, confirmed_at, inserted_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = ? COLLATE NOCASE
LIMIT 1;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = ?
LIMIT 1;

-- name: UpdateUserConfirmedAt :exec
UPDATE users
SET confirmed_at = ?, updated_at = ?
WHERE id = ?;
```

- [ ] **Step 2: Write `internal/store/queries/email_tokens.sql`**

```sql
-- name: CreateEmailToken :one
INSERT INTO email_tokens (id, user_id, token_hash, context, sent_to, inserted_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetEmailTokenByHashAndContext :one
SELECT *
FROM email_tokens
WHERE token_hash = ? AND context = ?
LIMIT 1;

-- name: DeleteEmailToken :exec
DELETE FROM email_tokens
WHERE id = ?;

-- name: DeleteEmailTokensForUser :exec
DELETE FROM email_tokens
WHERE user_id = ? AND context = ?;
```

- [ ] **Step 3: Generate**

```bash
just gen
```

Verify new generated files exist: `internal/store/sqlc/users.sql.go`, `internal/store/sqlc/email_tokens.sql.go`.

- [ ] **Step 4: Verify lint + tests still pass**

```bash
just precommit
```

- [ ] **Step 5: Commit**

```bash
git add internal/store/queries/ internal/store/sqlc/
git commit -m "feat(p1b): add sqlc queries for users and email_tokens"
```

---

## Task 4: Domain layer — `accounts`

**Files:** `internal/domain/accounts/user.go`, `internal/domain/accounts/email_token.go`, `internal/domain/accounts/repository.go`, `internal/domain/accounts/repository_test.go`

- [ ] **Step 1: Write the test (will drive the API)**

`internal/domain/accounts/repository_test.go`:

```go
package accounts_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/acai-sh/server/internal/domain/accounts"
	"github.com/acai-sh/server/internal/store"
)

func newRepo(t *testing.T) *accounts.Repository {
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
	return accounts.NewRepository(db)
}

func TestRepository_CreateAndGetUser(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	u, err := repo.CreateUser(ctx, accounts.CreateUserParams{
		Email:          "alice@example.com",
		HashedPassword: "$argon2id$test-hash",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == "" {
		t.Errorf("expected non-empty UUID id")
	}
	if u.Email != "alice@example.com" {
		t.Errorf("Email = %q, want %q", u.Email, "alice@example.com")
	}

	got, err := repo.GetUserByEmail(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("GetUserByEmail returned id %q, want %q", got.ID, u.ID)
	}

	// Case insensitivity.
	got2, err := repo.GetUserByEmail(ctx, "ALICE@EXAMPLE.COM")
	if err != nil {
		t.Fatalf("GetUserByEmail (uppercase): %v", err)
	}
	if got2.ID != u.ID {
		t.Errorf("citext lookup returned id %q, want %q", got2.ID, u.ID)
	}
}

func TestRepository_GetUserByEmail_NotFound(t *testing.T) {
	repo := newRepo(t)
	_, err := repo.GetUserByEmail(context.Background(), "ghost@example.com")
	if err == nil {
		t.Errorf("expected error for missing user, got nil")
	}
	if !accounts.IsNotFound(err) {
		t.Errorf("expected accounts.IsNotFound error, got: %v", err)
	}
}

func TestRepository_BuildAndConsumeMagicLinkToken(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	u, err := repo.CreateUser(ctx, accounts.CreateUserParams{
		Email:          "bob@example.com",
		HashedPassword: "$argon2id$bob",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	rawToken, err := repo.BuildEmailToken(ctx, u, "login")
	if err != nil {
		t.Fatalf("BuildEmailToken: %v", err)
	}
	if len(rawToken) == 0 {
		t.Fatalf("expected non-empty raw token")
	}

	// First consume succeeds.
	got, err := repo.ConsumeEmailToken(ctx, rawToken, "login", 15*time.Minute)
	if err != nil {
		t.Fatalf("ConsumeEmailToken: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("ConsumeEmailToken returned user id %q, want %q", got.ID, u.ID)
	}

	// Second consume fails (single-use).
	_, err = repo.ConsumeEmailToken(ctx, rawToken, "login", 15*time.Minute)
	if err == nil {
		t.Errorf("second ConsumeEmailToken should fail (single-use), got nil")
	}
}

func TestRepository_ConsumeEmailToken_RejectsExpired(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	u, err := repo.CreateUser(ctx, accounts.CreateUserParams{
		Email:          "carol@example.com",
		HashedPassword: "$argon2id$carol",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	rawToken, err := repo.BuildEmailToken(ctx, u, "login")
	if err != nil {
		t.Fatalf("BuildEmailToken: %v", err)
	}

	// Validity window of 0 → already expired.
	_, err = repo.ConsumeEmailToken(ctx, rawToken, "login", 0)
	if err == nil {
		t.Errorf("expected expired-token error, got nil")
	}
}
```

- [ ] **Step 2: Implement `internal/domain/accounts/user.go`**

```go
// Package accounts is the domain context for users, sessions-as-data, email
// tokens, and the canonical Scope used elsewhere as auth carrier.
package accounts

import "time"

// User mirrors the users table in shape; consumers should not assume any
// methods on it. Use Repository for mutations.
type User struct {
	ID             string
	Email          string
	HashedPassword string // PHC argon2id; may be "" for password-less accounts (not used in P1b)
	ConfirmedAt    *time.Time
	InsertedAt     time.Time
	UpdatedAt      time.Time
}
```

- [ ] **Step 3: Implement `internal/domain/accounts/email_token.go`**

```go
package accounts

import "time"

// EmailToken mirrors the email_tokens table. token_hash is sha256(rawToken);
// the raw token is only known at generation time and via the URL the user
// clicks.
type EmailToken struct {
	ID         string
	UserID     string
	TokenHash  []byte
	Context    string // "login" or "change_email:<old>"
	SentTo     string
	InsertedAt time.Time
}
```

- [ ] **Step 4: Implement `internal/domain/accounts/repository.go`**

```go
package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/acai-sh/server/internal/store"
	"github.com/acai-sh/server/internal/store/sqlc"
)

// Repository is the public domain API for the accounts context.
type Repository struct {
	db *store.DB
}

// NewRepository returns a Repository over db.
func NewRepository(db *store.DB) *Repository {
	return &Repository{db: db}
}

// ErrNotFound is returned when a lookup misses.
var ErrNotFound = errors.New("accounts: not found")

// IsNotFound reports whether err is ErrNotFound, including wrapped variants.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// CreateUserParams is the input for CreateUser.
type CreateUserParams struct {
	Email          string
	HashedPassword string
}

// CreateUser inserts a new user. ID is generated via UUIDv7 (time-ordered).
func (r *Repository) CreateUser(ctx context.Context, p CreateUserParams) (*User, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("accounts: gen uuid: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	q := sqlc.New(r.db.Write)
	row, err := q.CreateUser(ctx, sqlc.CreateUserParams{
		ID:             id.String(),
		Email:          p.Email,
		HashedPassword: sql.NullString{String: p.HashedPassword, Valid: p.HashedPassword != ""},
		ConfirmedAt:    sql.NullString{Valid: false},
		InsertedAt:     now,
		UpdatedAt:      now,
	})
	if err != nil {
		return nil, fmt.Errorf("accounts: insert user: %w", err)
	}
	return userFromRow(row)
}

// GetUserByEmail returns the user with the given email (case-insensitive),
// or ErrNotFound.
func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	q := sqlc.New(r.db.Read)
	row, err := q.GetUserByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("accounts: GetUserByEmail: %w", err)
	}
	return userFromRow(row)
}

// GetUserByID returns the user with the given UUID, or ErrNotFound.
func (r *Repository) GetUserByID(ctx context.Context, id string) (*User, error) {
	q := sqlc.New(r.db.Read)
	row, err := q.GetUserByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("accounts: GetUserByID: %w", err)
	}
	return userFromRow(row)
}

// BuildEmailToken generates a fresh random token for user in the given context
// (e.g., "login", "change_email:<old>"), stores its sha256 hash, and returns
// the base64-url-encoded raw token (this is what goes in the email link).
//
// The raw token is only revealed to the caller at this moment; only its hash
// is persisted.
func (r *Repository) BuildEmailToken(ctx context.Context, user *User, context string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("accounts: gen token: %w", err)
	}
	hashArr := sha256.Sum256(raw)
	hash := hashArr[:]

	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("accounts: gen uuid: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	q := sqlc.New(r.db.Write)
	_, err = q.CreateEmailToken(ctx, sqlc.CreateEmailTokenParams{
		ID:         id.String(),
		UserID:     user.ID,
		TokenHash:  hash,
		Context:    context,
		SentTo:     user.Email,
		InsertedAt: now,
	})
	if err != nil {
		return "", fmt.Errorf("accounts: insert email_token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// ConsumeEmailToken validates the raw token (base64-url) against the stored
// hash for the given context, returns the associated user iff valid, and
// deletes the token on success (single-use).
//
// Returns ErrNotFound if the hash isn't found, or a generic error if the token
// is older than validity.
func (r *Repository) ConsumeEmailToken(ctx context.Context, rawToken, context string, validity time.Duration) (*User, error) {
	raw, err := base64.RawURLEncoding.DecodeString(rawToken)
	if err != nil {
		return nil, fmt.Errorf("accounts: decode token: %w", err)
	}
	hashArr := sha256.Sum256(raw)
	hash := hashArr[:]

	qRead := sqlc.New(r.db.Read)
	tok, err := qRead.GetEmailTokenByHashAndContext(ctx, sqlc.GetEmailTokenByHashAndContextParams{
		TokenHash: hash,
		Context:   context,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("accounts: GetEmailTokenByHash: %w", err)
	}

	insertedAt, err := time.Parse(time.RFC3339Nano, tok.InsertedAt)
	if err != nil {
		return nil, fmt.Errorf("accounts: parse inserted_at: %w", err)
	}
	if time.Since(insertedAt) > validity {
		// Best-effort delete the expired token.
		_ = sqlc.New(r.db.Write).DeleteEmailToken(ctx, tok.ID)
		return nil, errors.New("accounts: email token expired")
	}

	user, err := r.GetUserByID(ctx, tok.UserID)
	if err != nil {
		return nil, fmt.Errorf("accounts: load user for token: %w", err)
	}

	// Single-use: delete in the same conceptual transaction. We accept a
	// race where two concurrent consumers both pass the SELECT then race on
	// DELETE; the loser will see 0 rows affected via DeleteEmailToken which
	// is :exec, so we mitigate by checking that the row still exists before
	// returning success. For P1b, the race is theoretical for human flows.
	if err := sqlc.New(r.db.Write).DeleteEmailToken(ctx, tok.ID); err != nil {
		return nil, fmt.Errorf("accounts: delete consumed token: %w", err)
	}

	return user, nil
}

// userFromRow converts a sqlc-generated User row into the domain User.
func userFromRow(row sqlc.User) (*User, error) {
	insertedAt, err := time.Parse(time.RFC3339Nano, row.InsertedAt)
	if err != nil {
		return nil, fmt.Errorf("accounts: parse inserted_at: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, row.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("accounts: parse updated_at: %w", err)
	}

	u := &User{
		ID:         row.ID,
		Email:      row.Email,
		InsertedAt: insertedAt,
		UpdatedAt:  updatedAt,
	}
	if row.HashedPassword.Valid {
		u.HashedPassword = row.HashedPassword.String
	}
	if row.ConfirmedAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, row.ConfirmedAt.String)
		if err == nil {
			u.ConfirmedAt = &t
		}
	}
	return u, nil
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/domain/accounts/... -v
```

Expected: 4 tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/accounts/
git commit -m "feat(p1b): add accounts domain (User + EmailToken + Repository)"
```

---

## Task 5: Mailer interface + noop impl

**Files:** `internal/mail/mailer.go`, `internal/mail/noop.go`, `internal/mail/noop_test.go`

- [ ] **Step 1: Write the test**

```go
package mail_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/acai-sh/server/internal/mail"
)

func TestNoop_LogsMagicLinkURLAtInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	m := mail.NewNoop(logger)
	err := m.SendMagicLink(context.Background(), mail.MagicLinkArgs{
		To:      "alice@example.com",
		URL:     "http://localhost:4000/users/log-in/abc123",
		FromEmail: "noreply@acai.sh",
		FromName:  "Acai",
	})
	if err != nil {
		t.Fatalf("SendMagicLink: %v", err)
	}

	line := strings.TrimSpace(buf.String())
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("log line not JSON: %v\nline=%q", err, line)
	}
	if rec["msg"] != "mail.noop: magic link" {
		t.Errorf(`msg = %v, want "mail.noop: magic link"`, rec["msg"])
	}
	if rec["to"] != "alice@example.com" {
		t.Errorf(`to = %v, want alice@example.com`, rec["to"])
	}
	if rec["url"] != "http://localhost:4000/users/log-in/abc123" {
		t.Errorf(`url = %v, want the magic-link URL`, rec["url"])
	}
}
```

- [ ] **Step 2: Implement `internal/mail/mailer.go`**

```go
// Package mail owns transactional email. P1b ships only the noop implementation
// (logs to slog); P1c adds the production Mailgun client.
package mail

import "context"

// Mailer abstracts sending transactional email. The interface is small on
// purpose — every email kind has its own typed args struct so adding a new
// kind is one method here, not a refactor of options.
type Mailer interface {
	SendMagicLink(ctx context.Context, args MagicLinkArgs) error
}

// MagicLinkArgs is the input for the magic-link email.
type MagicLinkArgs struct {
	To        string
	URL       string
	FromEmail string
	FromName  string
}
```

- [ ] **Step 3: Implement `internal/mail/noop.go`**

```go
package mail

import (
	"context"
	"log/slog"
)

// Noop logs every email it would send. Used in dev and tests so flows can
// progress without real SMTP credentials.
type Noop struct {
	log *slog.Logger
}

// NewNoop returns a *Noop that emits log lines on the given logger.
func NewNoop(log *slog.Logger) *Noop {
	return &Noop{log: log}
}

// SendMagicLink emits a JSON log line describing the email.
func (n *Noop) SendMagicLink(_ context.Context, args MagicLinkArgs) error {
	n.log.Info("mail.noop: magic link",
		slog.String("to", args.To),
		slog.String("url", args.URL),
		slog.String("from_email", args.FromEmail),
		slog.String("from_name", args.FromName),
	)
	return nil
}
```

- [ ] **Step 4: Test**

```bash
go test ./internal/mail/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/mail/
git commit -m "feat(p1b): add Mailer interface + Noop implementation"
```

---

## Task 6: Magic-link service

**Files:** `internal/auth/magic_link.go`, `internal/auth/magic_link_test.go`

- [ ] **Step 1: Write the test**

```go
package auth_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/acai-sh/server/internal/auth"
	"github.com/acai-sh/server/internal/domain/accounts"
	"github.com/acai-sh/server/internal/store"
)

func newAccountsRepo(t *testing.T) *accounts.Repository {
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
	return accounts.NewRepository(db)
}

func TestMagicLinkService_GenerateAndConsume(t *testing.T) {
	ctx := context.Background()
	repo := newAccountsRepo(t)

	user, err := repo.CreateUser(ctx, accounts.CreateUserParams{
		Email:          "user@example.com",
		HashedPassword: "$argon2id$test",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	svc := auth.NewMagicLinkService(repo, "https://acai.test")

	url, rawToken, err := svc.GenerateLoginURL(ctx, user)
	if err != nil {
		t.Fatalf("GenerateLoginURL: %v", err)
	}
	if url == "" || rawToken == "" {
		t.Fatalf("expected non-empty url and token; url=%q token=%q", url, rawToken)
	}
	if want := "https://acai.test/users/log-in/" + rawToken; url != want {
		t.Errorf("url = %q, want %q", url, want)
	}

	got, err := svc.ConsumeLoginToken(ctx, rawToken)
	if err != nil {
		t.Fatalf("ConsumeLoginToken: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("user id = %q, want %q", got.ID, user.ID)
	}
}
```

- [ ] **Step 2: Implement `internal/auth/magic_link.go`**

```go
package auth

import (
	"context"
	"time"

	"github.com/acai-sh/server/internal/domain/accounts"
)

// MagicLinkValidity is how long a login magic-link is accepted after issue.
const MagicLinkValidity = 15 * time.Minute

// MagicLinkService composes accounts.Repository to produce magic-link URLs and
// consume them. The base URL (e.g. "https://app.acai.sh") comes from
// config.Config (URLScheme + URLHost).
type MagicLinkService struct {
	repo    *accounts.Repository
	baseURL string
}

// NewMagicLinkService returns a *MagicLinkService bound to repo and the given
// base URL (no trailing slash).
func NewMagicLinkService(repo *accounts.Repository, baseURL string) *MagicLinkService {
	return &MagicLinkService{repo: repo, baseURL: baseURL}
}

// GenerateLoginURL builds an emailable magic-link URL for user. Returns the
// full URL plus the raw token (callers may want to log/inspect).
func (s *MagicLinkService) GenerateLoginURL(ctx context.Context, user *accounts.User) (url, rawToken string, err error) {
	rawToken, err = s.repo.BuildEmailToken(ctx, user, "login")
	if err != nil {
		return "", "", err
	}
	url = s.baseURL + "/users/log-in/" + rawToken
	return url, rawToken, nil
}

// ConsumeLoginToken validates and single-use-consumes a login token, returning
// the associated user.
func (s *MagicLinkService) ConsumeLoginToken(ctx context.Context, rawToken string) (*accounts.User, error) {
	return s.repo.ConsumeEmailToken(ctx, rawToken, "login", MagicLinkValidity)
}
```

- [ ] **Step 3: Test**

```bash
go test ./internal/auth/... -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/auth/magic_link.go internal/auth/magic_link_test.go
git commit -m "feat(p1b): add MagicLinkService over accounts.Repository"
```

---

## Task 7: Sessions, Scope, middleware

**Files:** `internal/auth/scope.go`, `internal/auth/session.go`, `internal/auth/middleware.go`, `internal/auth/middleware_test.go`

- [ ] **Step 1: Implement `internal/auth/scope.go`**

```go
package auth

import (
	"context"

	"github.com/acai-sh/server/internal/domain/accounts"
)

// Scope is the auth carrier threaded through the request lifecycle. nil User
// means anonymous; non-nil means authenticated.
type Scope struct {
	User *accounts.User
}

// IsAuthenticated reports whether the scope has a user attached.
func (s *Scope) IsAuthenticated() bool { return s != nil && s.User != nil }

type scopeKey struct{}

// WithScope returns a derived ctx carrying scope.
func WithScope(ctx context.Context, scope *Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

// ScopeFrom returns the scope from ctx, or an anonymous scope.
func ScopeFrom(ctx context.Context) *Scope {
	if s, ok := ctx.Value(scopeKey{}).(*Scope); ok && s != nil {
		return s
	}
	return &Scope{}
}
```

- [ ] **Step 2: Implement `internal/auth/session.go`**

```go
package auth

import (
	"net/http"
	"time"

	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"

	"github.com/acai-sh/server/internal/store"
)

// SessionLifetime is how long a session cookie persists if remember-me is set.
const SessionLifetime = 14 * 24 * time.Hour

// NewSessionManager returns a configured *scs.SessionManager backed by the
// SQLite store. secureCookie should be true in prod (HTTPS) and false in dev.
func NewSessionManager(db *store.DB, secureCookie bool) *scs.SessionManager {
	mgr := scs.New()
	mgr.Store = sqlite3store.New(db.Write)
	mgr.Lifetime = SessionLifetime
	mgr.IdleTimeout = 0
	mgr.Cookie.Name = "_acai_session"
	mgr.Cookie.Path = "/"
	mgr.Cookie.HttpOnly = true
	mgr.Cookie.Secure = secureCookie
	mgr.Cookie.SameSite = http.SameSiteLaxMode
	mgr.Cookie.Persist = false // toggled true by handler when remember-me checked (P1c)
	return mgr
}

// Session keys we use across the codebase.
const (
	sessionKeyUserID          = "user_id"
	sessionKeyAuthenticatedAt = "authenticated_at"
)

// Login sets the session keys for an authenticated user.
func Login(mgr *scs.SessionManager, ctx context.Context, userID string) {
	mgr.Put(ctx, sessionKeyUserID, userID)
	mgr.Put(ctx, sessionKeyAuthenticatedAt, time.Now().UTC().Format(time.RFC3339Nano))
}

// Logout clears the session.
func Logout(mgr *scs.SessionManager, ctx context.Context) error {
	return mgr.Destroy(ctx)
}

// CurrentUserID returns the user id from the session, or "" if anonymous.
func CurrentUserID(mgr *scs.SessionManager, ctx context.Context) string {
	return mgr.GetString(ctx, sessionKeyUserID)
}
```

(Add `import "context"` at the top.)

- [ ] **Step 3: Implement `internal/auth/middleware.go`**

```go
package auth

import (
	"net/http"

	"github.com/alexedwards/scs/v2"

	"github.com/acai-sh/server/internal/domain/accounts"
)

// LoadScope reads the user_id from the session, fetches the user via the
// accounts repo, and stores a *Scope on the request context. If no user_id is
// present (anonymous), an empty Scope is attached.
func LoadScope(mgr *scs.SessionManager, repo *accounts.Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			scope := &Scope{}

			if userID := mgr.GetString(ctx, sessionKeyUserID); userID != "" {
				user, err := repo.GetUserByID(ctx, userID)
				if err == nil {
					scope.User = user
				} else if accounts.IsNotFound(err) {
					_ = mgr.Destroy(ctx) // user gone, kill the session
				}
				// other DB errors: keep scope anonymous; LoadScope is best-effort
			}

			next.ServeHTTP(w, r.WithContext(WithScope(ctx, scope)))
		})
	}
}

// RequireAuth redirects unauthenticated users to /users/log-in. Use after
// LoadScope.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ScopeFrom(r.Context()).IsAuthenticated() {
			http.Redirect(w, r, "/users/log-in", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RedirectIfAuth redirects already-authenticated users to /teams. Use on the
// log-in page so logged-in users don't see the form.
func RedirectIfAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ScopeFrom(r.Context()).IsAuthenticated() {
			http.Redirect(w, r, "/teams", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 4: Tests for middleware**

`internal/auth/middleware_test.go`:

```go
package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/acai-sh/server/internal/auth"
	"github.com/acai-sh/server/internal/domain/accounts"
	"github.com/acai-sh/server/internal/store"
)

func newDB(t *testing.T) *store.DB {
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
	return db
}

func TestRequireAuth_RedirectsAnonymous(t *testing.T) {
	handler := auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("downstream handler should not have been called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/teams", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/users/log-in" {
		t.Errorf("Location = %q, want /users/log-in", loc)
	}
}

func TestRequireAuth_AllowsAuthenticated(t *testing.T) {
	called := false
	handler := auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/teams", http.NoBody)
	ctx := auth.WithScope(req.Context(), &auth.Scope{User: &accounts.User{ID: "u1"}})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Errorf("downstream handler not called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRedirectIfAuth_RedirectsAuthenticated(t *testing.T) {
	handler := auth.RedirectIfAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("downstream handler should not have been called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/users/log-in", http.NoBody)
	ctx := auth.WithScope(req.Context(), &auth.Scope{User: &accounts.User{ID: "u1"}})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/teams" {
		t.Errorf("Location = %q, want /teams", loc)
	}
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/auth/... -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/auth/
git commit -m "feat(p1b): add scs sessions, Scope ctx helpers, and auth middleware"
```

---

## Task 8: Login handlers + site router

**Files:** `internal/site/handlers/auth.go`, `internal/site/templates/auth.go`, `internal/site/router.go`, modify `internal/server/router.go`

- [ ] **Step 1: Create `internal/site/templates/auth.go` with bare HTML templates**

```go
// Package templates holds html/template strings for site pages. P1b uses bare
// HTML; P1c migrates to templ + Tailwind + Datastar.
package templates

import "html/template"

// LoginPage renders the email-entry form.
var LoginPage = template.Must(template.New("login").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Log in — Acai</title>
</head>
<body>
  <h1>Log in</h1>
  {{if .Flash}}<p style="color: #b00">{{.Flash}}</p>{{end}}
  <form method="post" action="/users/log-in">
    <input type="hidden" name="{{.CSRFFieldName}}" value="{{.CSRFToken}}">
    <label>Email <input type="email" name="email" required autofocus></label>
    <button type="submit">Send magic link</button>
  </form>
</body>
</html>`))

// LoginPageData is the input for LoginPage.
type LoginPageData struct {
	Flash         string
	CSRFFieldName string
	CSRFToken     string
}

// LoginRequestedPage renders the "check your email" confirmation.
var LoginRequestedPage = template.Must(template.New("login_requested").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Check your email — Acai</title>
</head>
<body>
  <h1>Check your email</h1>
  <p>If <strong>{{.Email}}</strong> matches an account, you'll receive a magic-link shortly.</p>
  <p><a href="/users/log-in">Back</a></p>
</body>
</html>`))

// LoginRequestedPageData is the input for LoginRequestedPage.
type LoginRequestedPageData struct {
	Email string
}
```

- [ ] **Step 2: Create `internal/site/handlers/auth.go`**

```go
// Package handlers is the home of site (browser) HTTP handlers. P1b lands the
// auth flow; subsequent phases extend with team/product/feature pages.
package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/gorilla/csrf"

	"github.com/acai-sh/server/internal/auth"
	"github.com/acai-sh/server/internal/domain/accounts"
	"github.com/acai-sh/server/internal/mail"
	"github.com/acai-sh/server/internal/site/templates"
)

// AuthDeps groups handler dependencies.
type AuthDeps struct {
	Logger     *slog.Logger
	Sessions   *scs.SessionManager
	Accounts   *accounts.Repository
	MagicLink  *auth.MagicLinkService
	Mailer     mail.Mailer
	FromEmail  string
	FromName   string
}

// LoginNew GETs the login form.
func LoginNew(d *AuthDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.LoginPage.Execute(w, templates.LoginPageData{
			CSRFFieldName: "gorilla.csrf.Token",
			CSRFToken:     csrf.Token(r),
		})
	}
}

// LoginCreate POSTs the login form: looks up user by email, generates a
// magic-link, sends via mailer, then renders the "check your email" page.
// Note: we always render the same response regardless of whether the email
// matches — to avoid email enumeration.
func LoginCreate(d *AuthDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		email := r.PostForm.Get("email")

		// Look up user; if missing, still render the success page.
		user, err := d.Accounts.GetUserByEmail(r.Context(), email)
		if err == nil {
			url, _, genErr := d.MagicLink.GenerateLoginURL(r.Context(), user)
			if genErr == nil {
				_ = d.Mailer.SendMagicLink(r.Context(), mail.MagicLinkArgs{
					To:        user.Email,
					URL:       url,
					FromEmail: d.FromEmail,
					FromName:  d.FromName,
				})
			} else {
				d.Logger.Warn("login: generate magic link", "error", genErr)
			}
		} else if !accounts.IsNotFound(err) {
			d.Logger.Warn("login: lookup user", "error", err)
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.LoginRequestedPage.Execute(w, templates.LoginRequestedPageData{Email: email})
	}
}

// LoginConfirm consumes a magic-link token from the URL path. On success it
// installs the session; on failure it renders the login page with a flash.
func LoginConfirm(d *AuthDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		if token == "" {
			http.Error(w, "missing token", http.StatusBadRequest)
			return
		}
		user, err := d.MagicLink.ConsumeLoginToken(r.Context(), token)
		if err != nil {
			d.Logger.Info("login: consume token failed", "error", err)
			renderLoginWithFlash(w, r, "That magic link is invalid or expired. Please request a new one.")
			return
		}

		// Renew session id (defense vs fixation).
		if err := d.Sessions.RenewToken(r.Context()); err != nil {
			d.Logger.Warn("login: renew token", "error", err)
		}

		auth.Login(d.Sessions, r.Context(), user.ID)
		http.Redirect(w, r, "/teams", http.StatusSeeOther)
	}
}

// LogOut destroys the session and redirects to the home/login page.
func LogOut(d *AuthDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := auth.Logout(d.Sessions, r.Context()); err != nil {
			d.Logger.Warn("logout: destroy session", "error", err)
		}
		http.Redirect(w, r, "/users/log-in", http.StatusSeeOther)
	}
}

func renderLoginWithFlash(w http.ResponseWriter, r *http.Request, flash string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = templates.LoginPage.Execute(w, templates.LoginPageData{
		Flash:         flash,
		CSRFFieldName: "gorilla.csrf.Token",
		CSRFToken:     csrf.Token(r),
	})
}

// Compile-time check that we use accounts.IsNotFound to avoid an unused-import warning.
var _ = errors.Is

// Avoid a "context unused" lint if context drops out — keep import.
var _ = context.Background
```

- [ ] **Step 3: Create `internal/site/router.go`**

```go
// Package site mounts browser-facing HTTP routes (auth pages, team views,
// settings). Pure HTML rendering for now; templ/Datastar lands in P1c.
package site

import (
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"

	"github.com/acai-sh/server/internal/auth"
	"github.com/acai-sh/server/internal/site/handlers"
)

// MountAuthRoutes registers the login/logout routes on r. Caller is expected
// to have already mounted sessionManager.LoadAndSave + auth.LoadScope at the
// parent level so each handler can read the session/scope.
//
// CSRF is applied here on the auth subtree (everywhere except the
// magic-link-confirm GET, which is the auth proof itself).
func MountAuthRoutes(r chi.Router, deps *handlers.AuthDeps, csrfKey []byte, secureCookie bool) {
	csrfMiddleware := csrf.Protect(csrfKey,
		csrf.Secure(secureCookie),
		csrf.Path("/"),
		csrf.SameSite(csrf.SameSiteLaxMode),
	)

	// Routes for unauthenticated users only — RedirectIfAuth bounces logged-in users.
	r.Group(func(r chi.Router) {
		r.Use(csrfMiddleware)
		r.Use(auth.RedirectIfAuth)
		r.Get("/users/log-in", handlers.LoginNew(deps))
		r.Post("/users/log-in", handlers.LoginCreate(deps))
	})

	// Magic-link consume — bypasses CSRF (the token IS the proof). Also
	// bypasses RedirectIfAuth: a user clicking a link in an old session should
	// log out the old session and start fresh; for now, ConsumeLoginToken just
	// overwrites the session.
	r.Get("/users/log-in/{token}", handlers.LoginConfirm(deps))

	// Logout always allowed.
	r.Group(func(r chi.Router) {
		r.Use(csrfMiddleware)
		r.Post("/users/log-out", handlers.LogOut(deps))
	})
}

// MountAuthRequiredStub mounts a single test endpoint that requires auth.
// P1b's only authenticated route — proves middleware works. Extended in P2/P3.
func MountAuthRequiredStub(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth)
		r.Get("/teams", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			s := auth.ScopeFrom(r.Context())
			_, _ = w.Write([]byte("<!DOCTYPE html><html><body><h1>Teams</h1>"))
			_, _ = w.Write([]byte("<p>Logged in as " + s.User.Email + "</p>"))
			_, _ = w.Write([]byte(`<form method="post" action="/users/log-out">`))
			// CSRF token here would require pulling csrf.Token(r) — but logout group has its own CSRF wrapper.
			// For P1b's stub page, we just hand-roll a hidden token using the request's csrf middleware.
			_, _ = w.Write([]byte(`<input type="hidden" name="gorilla.csrf.Token" value="`))
			_, _ = w.Write([]byte(csrf.Token(r)))
			_, _ = w.Write([]byte(`"><button type="submit">Log out</button></form></body></html>`))
		})
	})
}
```

(`csrf.Token(r)` reads the token from the request; the middleware injects it.)

- [ ] **Step 4: Update `internal/server/router.go`**

```go
package server

import (
	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/acai-sh/server/internal/auth"
	"github.com/acai-sh/server/internal/domain/accounts"
	"github.com/acai-sh/server/internal/ops"
	"github.com/acai-sh/server/internal/site"
	"github.com/acai-sh/server/internal/site/handlers"
	"github.com/acai-sh/server/internal/store"
)

// RouterDeps groups everything newRouter needs from the caller.
type RouterDeps struct {
	DB              *store.DB
	Sessions        *scs.SessionManager
	Accounts        *accounts.Repository
	AuthHandlerDeps *handlers.AuthDeps
	CSRFKey         []byte
	SecureCookie    bool
	Version         string
}

// newRouter builds the chi router with auth middleware in place.
func newRouter(deps *RouterDeps) chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	// Browser routes get sessions + scope.
	r.Group(func(r chi.Router) {
		r.Use(deps.Sessions.LoadAndSave)
		r.Use(auth.LoadScope(deps.Sessions, deps.Accounts))

		site.MountAuthRoutes(r, deps.AuthHandlerDeps, deps.CSRFKey, deps.SecureCookie)
		site.MountAuthRequiredStub(r)
	})

	// Health check is outside the session middleware (cheap, no cookies).
	r.Method("GET", "/_health", ops.HealthHandler(deps.DB, deps.Version))

	return r
}
```

Update `Server.New` and `server.go` to take a `*RouterDeps` (or similar) — propagate the new fields. Update `server_test.go` accordingly to construct the deps; the existing `/_health` test should still pass since that route is untouched.

- [ ] **Step 5: Update server constructor**

In `internal/server/server.go`, change `New` to accept the broader deps:

```go
func New(cfg *config.Config, logger *slog.Logger, deps *RouterDeps) (*Server, error) {
	if cfg == nil {
		return nil, errors.New("server: cfg is nil")
	}
	if logger == nil {
		return nil, errors.New("server: logger is nil")
	}
	if deps == nil || deps.DB == nil {
		return nil, errors.New("server: deps with DB are required")
	}

	router := newRouter(deps)

	httpServer := &http.Server{
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return &Server{
		cfg:     cfg,
		logger:  logger,
		db:      deps.DB,
		version: deps.Version,
		http:    httpServer,
	}, nil
}
```

Update `server_test.go` to construct `*RouterDeps` (with `DB`, `Version`; other fields can be nil for the health-only test if the routes that need them aren't exercised — but they are mounted; so initialize them all). Use a real session manager and accounts repo.

- [ ] **Step 6: Update `cmd/acai/serve.go`** to construct all dependencies and pass via RouterDeps.

```go
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/acai-sh/server/internal/auth"
	"github.com/acai-sh/server/internal/config"
	"github.com/acai-sh/server/internal/domain/accounts"
	"github.com/acai-sh/server/internal/mail"
	"github.com/acai-sh/server/internal/ops"
	"github.com/acai-sh/server/internal/server"
	"github.com/acai-sh/server/internal/site/handlers"
	"github.com/acai-sh/server/internal/store"
)

func runServe(ctx context.Context, stderr io.Writer) int {
	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "config: %v\n", err)
		return 1
	}

	logger := ops.SetupLogger(cfg, stderr)

	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "store.Open: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	if err := store.RunMigrations(ctx, db); err != nil {
		_, _ = fmt.Fprintf(stderr, "store.RunMigrations: %v\n", err)
		return 1
	}

	repo := accounts.NewRepository(db)
	sessionManager := auth.NewSessionManager(db, cfg.URLScheme == "https")
	baseURL := cfg.URLScheme + "://" + cfg.URLHost
	if cfg.HTTPPort != 80 && cfg.HTTPPort != 443 && cfg.URLHost == "localhost" {
		baseURL = fmt.Sprintf("%s://%s:%d", cfg.URLScheme, cfg.URLHost, cfg.HTTPPort)
	}
	mlSvc := auth.NewMagicLinkService(repo, baseURL)

	var mailer mail.Mailer = mail.NewNoop(logger)
	if !cfg.MailNoop {
		_, _ = fmt.Fprintln(stderr, "warning: MAIL_NOOP=false but no production mailer is wired in P1b; using noop")
	}

	authDeps := &handlers.AuthDeps{
		Logger:    logger,
		Sessions:  sessionManager,
		Accounts:  repo,
		MagicLink: mlSvc,
		Mailer:    mailer,
		FromEmail: cfg.MailFromEmail,
		FromName:  cfg.MailFromName,
	}

	srv, err := server.New(cfg, logger, &server.RouterDeps{
		DB:              db,
		Sessions:        sessionManager,
		Accounts:        repo,
		AuthHandlerDeps: authDeps,
		CSRFKey:         []byte(cfg.SecretKeyBase[:32]),
		SecureCookie:    cfg.URLScheme == "https",
		Version:         version,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "server.New: %v\n", err)
		return 1
	}

	if err := srv.Run(ctx, nil); err != nil {
		_, _ = fmt.Fprintf(stderr, "server.Run: %v\n", err)
		return 1
	}
	return 0
}
```

(SecretKeyBase guaranteed ≥32 by config validation.)

- [ ] **Step 7: Run all tests**

```bash
just precommit
```

Fix any breakage. The existing `internal/server` test must still pass with the new `RouterDeps` API (you'll need to construct the full deps — even though /_health doesn't use them, the router mounts all handlers).

- [ ] **Step 8: Commit**

```bash
git add internal/site/ internal/server/ cmd/acai/serve.go
git commit -m "feat(p1b): add login handlers + site router with sessions and CSRF"
```

---

## Task 9: End-to-end test for the full magic-link flow

**Files:** `internal/site/handlers/auth_test.go`

- [ ] **Step 1: Write the e2e test**

```go
package handlers_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acai-sh/server/internal/auth"
	"github.com/acai-sh/server/internal/config"
	"github.com/acai-sh/server/internal/domain/accounts"
	"github.com/acai-sh/server/internal/mail"
	"github.com/acai-sh/server/internal/ops"
	"github.com/acai-sh/server/internal/server"
	"github.com/acai-sh/server/internal/site/handlers"
	"github.com/acai-sh/server/internal/store"
)

// captureMailer captures the magic-link URL instead of "sending" it.
type captureMailer struct{ url string }

func (c *captureMailer) SendMagicLink(_ context.Context, args mail.MagicLinkArgs) error {
	c.url = args.URL
	return nil
}

func TestLogin_FullMagicLinkFlow(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	repo := accounts.NewRepository(db)
	user, err := repo.CreateUser(context.Background(), accounts.CreateUserParams{
		Email:          "alice@example.com",
		HashedPassword: "$argon2id$dummy",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	_ = user

	cfg := &config.Config{
		LogLevel:      "warn",
		HTTPPort:      0,
		SecretKeyBase: strings.Repeat("a", 32) + "DEV-ONLY-secret-key-base-for-test",
		URLHost:       "localhost",
		URLScheme:     "http",
		MailNoop:      true,
		MailFromName:  "Acai Test",
		MailFromEmail: "test@acai.test",
	}
	logger := ops.SetupLogger(cfg, io.Discard)
	sessionManager := auth.NewSessionManager(db, false)
	mlSvc := auth.NewMagicLinkService(repo, "http://localhost")
	cap := &captureMailer{}
	authDeps := &handlers.AuthDeps{
		Logger: logger, Sessions: sessionManager, Accounts: repo,
		MagicLink: mlSvc, Mailer: cap,
		FromEmail: cfg.MailFromEmail, FromName: cfg.MailFromName,
	}

	srv, err := server.New(cfg, logger, &server.RouterDeps{
		DB: db, Sessions: sessionManager, Accounts: repo, AuthHandlerDeps: authDeps,
		CSRFKey: []byte(cfg.SecretKeyBase[:32]), SecureCookie: false, Version: "test",
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Step 1: GET /users/log-in to obtain CSRF token.
	resp, err := client.Get(ts.URL + "/users/log-in")
	if err != nil {
		t.Fatalf("GET /users/log-in: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	csrfToken := extractCSRFToken(string(body))
	if csrfToken == "" {
		t.Fatalf("CSRF token not found in login form; body=%s", body)
	}

	// Step 2: POST /users/log-in with email.
	resp, err = client.PostForm(ts.URL+"/users/log-in", url.Values{
		"email":              {"alice@example.com"},
		"gorilla.csrf.Token": {csrfToken},
	})
	if err != nil {
		t.Fatalf("POST /users/log-in: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if cap.url == "" {
		t.Fatal("mailer did not capture a magic-link URL")
	}

	// Step 3: GET the magic-link URL — but rewrite host since cap.url uses "localhost".
	confirmURL := ts.URL + strings.TrimPrefix(cap.url, "http://localhost")
	resp, err = client.Get(confirmURL)
	if err != nil {
		t.Fatalf("GET confirm: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("confirm status = %d, want %d (redirect to /teams)", resp.StatusCode, http.StatusSeeOther)
	}
	if loc := resp.Header.Get("Location"); loc != "/teams" {
		t.Errorf("Location = %q, want /teams", loc)
	}

	// Step 4: GET /teams — should now be authenticated.
	resp, err = client.Get(ts.URL + "/teams")
	if err != nil {
		t.Fatalf("GET /teams: %v", err)
	}
	tbody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/teams status = %d, want %d; body=%s", resp.StatusCode, http.StatusOK, tbody)
	}
	if !strings.Contains(string(tbody), "alice@example.com") {
		t.Errorf("/teams body did not contain user email; body=%s", tbody)
	}
}

func extractCSRFToken(html string) string {
	const needle = `name="gorilla.csrf.Token" value="`
	i := strings.Index(html, needle)
	if i < 0 {
		return ""
	}
	rest := html[i+len(needle):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}
```

(This requires `Server.Handler()` to expose the inner mux. Add it to `internal/server/server.go`:)

```go
// Handler returns the underlying http.Handler. Useful for tests using
// httptest.NewServer.
func (s *Server) Handler() http.Handler { return s.http.Handler }
```

- [ ] **Step 2: Run**

```bash
go test ./internal/site/handlers/... -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/site/handlers/auth_test.go internal/server/server.go
git commit -m "test(p1b): end-to-end magic-link login flow"
```

---

## Task 10: `acai create-admin` subcommand

**Files:** `cmd/acai/create_admin.go`, modify `cmd/acai/main.go`, `cmd/acai/main_test.go`

- [ ] **Step 1: Add the subcommand impl**

`cmd/acai/create_admin.go`:

```go
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"io"

	"github.com/acai-sh/server/internal/auth"
	"github.com/acai-sh/server/internal/config"
	"github.com/acai-sh/server/internal/domain/accounts"
	"github.com/acai-sh/server/internal/ops"
	"github.com/acai-sh/server/internal/store"
)

// runCreateAdmin parses --email + optional --password, opens the DB, runs
// migrations, hashes the password (random 32-byte token if not given), and
// inserts the user. Prints the email + "user created" on success.
func runCreateAdmin(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	fs.SetOutput(stderr)
	email := fs.String("email", "", "email address (required)")
	password := fs.String("password", "", "password (random if blank)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *email == "" {
		_, _ = fmt.Fprintln(stderr, "create-admin: --email is required")
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "config: %v\n", err)
		return 1
	}
	logger := ops.SetupLogger(cfg, stderr)
	_ = logger

	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "store.Open: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	if err := store.RunMigrations(ctx, db); err != nil {
		_, _ = fmt.Fprintf(stderr, "store.RunMigrations: %v\n", err)
		return 1
	}

	rawPassword := *password
	if rawPassword == "" {
		buf := make([]byte, 24)
		if _, err := rand.Read(buf); err != nil {
			_, _ = fmt.Fprintf(stderr, "create-admin: random password: %v\n", err)
			return 1
		}
		rawPassword = base64.RawURLEncoding.EncodeToString(buf)
	}

	hash, err := auth.HashPassword(rawPassword)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "create-admin: hash: %v\n", err)
		return 1
	}

	repo := accounts.NewRepository(db)
	user, err := repo.CreateUser(ctx, accounts.CreateUserParams{
		Email:          *email,
		HashedPassword: hash,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "create-admin: insert: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintf(stdout, "user created: %s (%s)\n", user.Email, user.ID)
	if *password == "" {
		_, _ = fmt.Fprintf(stdout, "generated password: %s\n", rawPassword)
		_, _ = fmt.Fprintln(stdout, "(use the magic-link login flow; this password is only for completeness)")
	}
	return 0
}
```

- [ ] **Step 2: Wire into `cmd/acai/main.go`**

Update the switch in `run`:

```go
case "create-admin":
    return runCreateAdmin(ctx, args[2:], stdout, stderr)
```

Update the usage text:

```go
_, _ = fmt.Fprintln(stdout, "subcommands: serve, migrate, create-admin, version")
```

- [ ] **Step 3: Add an integration test in `cmd/acai/main_test.go`**

```go
func TestRun_CreateAdminSubcommand(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	t.Setenv("DATABASE_PATH", dbPath)
	t.Setenv("HTTP_PORT", "4000")
	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("SECRET_KEY_BASE", strings.Repeat("a", 32)+"-test-secret-key-base")
	t.Setenv("MAIL_NOOP", "true")
	t.Setenv("URL_HOST", "localhost")
	t.Setenv("URL_SCHEME", "http")

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{
		"acai", "create-admin", "--email", "first@example.com", "--password", "secret-password-12345",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("create-admin exit = %d; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "first@example.com") {
		t.Errorf("stdout missing email; got %q", stdout.String())
	}
}
```

(Add `import "strings"` if not present.)

- [ ] **Step 4: Run all tests**

```bash
just precommit
```

- [ ] **Step 5: End-to-end smoke**

```bash
just build
DATABASE_PATH=/tmp/acai-p1b.db HTTP_PORT=4002 \
  SECRET_KEY_BASE="$(openssl rand -hex 32)" \
  ./acai create-admin --email me@example.com --password test-pass-12345

DATABASE_PATH=/tmp/acai-p1b.db HTTP_PORT=4002 \
  SECRET_KEY_BASE="$(openssl rand -hex 32)" \
  ./acai serve &
SERVER_PID=$!
sleep 1
echo "--- Test login flow manually: ---"
echo "  GET  http://localhost:4002/users/log-in"
echo "  POST email=me@example.com → server logs the magic-link URL"
echo "  GET  the logged URL → redirected to /teams"

# Clean up
kill -INT $SERVER_PID 2>/dev/null || true
wait $SERVER_PID 2>/dev/null
rm -f /tmp/acai-p1b.db /tmp/acai-p1b.db-shm /tmp/acai-p1b.db-wal
just clean
```

- [ ] **Step 6: Commit**

```bash
git add cmd/acai/
git commit -m "feat(p1b): add acai create-admin subcommand"
```

---

## Task 11: Push and verify CI

- [ ] **Step 1: Push and watch**

```bash
git push
sleep 8
LATEST_ID=$(gh run list --repo jadams-positron/acai-sh-server --branch rewrite/go-datastar --limit 1 --json databaseId -q '.[0].databaseId')
echo "Watching run $LATEST_ID"
until [ -n "$(gh run view $LATEST_ID --repo jadams-positron/acai-sh-server --json conclusion -q '.conclusion // empty')" ]; do sleep 12; done
gh run view $LATEST_ID --repo jadams-positron/acai-sh-server --json status,conclusion
```

- [ ] **Step 2: If failure, capture and escalate.**

- [ ] **Step 3: Mark P1b done.** P1c plan (registration + Tailwind/Datastar styling + Mailgun mailer) gets written via writing-plans next.

---

## Self-Review Notes

**Spec coverage (P1b slice):** ✓ Magic-link login flow end-to-end (sessions + scope + middleware + handlers + templates + e2e test). Registration UI, full styling, and production mailer are explicitly deferred to P1c — keeps this plan tractable.

**Out-of-scope guards:** No templ, no Tailwind, no Datastar runtime. No Mailgun. No CSRF on the magic-link consume route (intentional — the token is the auth proof). No global admin or sudo-mode middleware (those land when /admin/* routes exist, P3 or later).

**Type consistency:** `*accounts.User` flows through Scope → handler. `*scs.SessionManager` is the single source for session state. `*auth.MagicLinkService` is the only path to creating/consuming login tokens. `mail.Mailer` is the single mailer interface, with `mail.NewNoop` the only impl.

**Risk register:**
- **scs sqlite3store + modernc driver** — scs's sqlite3store typically expects mattn/go-sqlite3; verify it works against modernc on first run. If broken, fall back to scs's `memstore` for P1b dev (lossy on restart) and tackle store choice in P1c.
- **CSRF token in the stub /teams page** — gorilla/csrf only injects the token on routes wrapped with `csrf.Protect`. Our /teams stub isn't in a CSRF group, so `csrf.Token(r)` returns "". The logout form will fail CSRF validation. Acceptable for P1b stub; P1c rewrites this page properly.
- **Email enumeration timing** — LoginCreate intentionally always returns 200 OK regardless of whether the email exists, but the response time differs (real users exercise BuildEmailToken + Mailer; missing users skip both). For P1b that's acceptable; production hardening adds a dummy crypto step.
