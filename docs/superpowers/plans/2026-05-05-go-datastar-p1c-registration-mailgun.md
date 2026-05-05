# Go + Datastar Rewrite — Phase 1c: Registration + Production Mailer

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** A new user can self-register at `/users/register` (email-only — passwordless) and receive a real email via Mailgun's HTTP API. Magic-link login still works for existing users. Mailer selection (`Noop` vs `Mailgun`) is config-driven via `MAIL_NOOP`. Visual rewrite to templ + Tailwind + Datastar stays deferred to **P1d**.

**Architecture:** New `/users/register` POST handler creates a user with `confirmed_at=null` and sends a magic-link to confirm. Confirmation = first successful magic-link login (sets `confirmed_at`). Mailer interface gains a single new impl: `mail.Mailgun` over `github.com/mailgun/mailgun-go/v4`. Construction of the right impl moves into `mail.NewFromConfig(*Config, *slog.Logger)`.

**Tech Stack:** `github.com/mailgun/mailgun-go/v4` for transactional email. Existing Tailwind/templ/Datastar are still NOT introduced in P1c.

**Reference:** Spec `docs/superpowers/specs/2026-05-05-go-datastar-rewrite-design.md` §2 (Mailer interface), §7 (auth). Branch `rewrite/go-datastar` after P1b at commit `cfca0ef`.

---

## Scope decisions

- **No password during registration.** Accounts are created with `hashed_password=NULL`. Magic-link is the only auth path. (Password is still on the schema for legacy-import compatibility from Phoenix.)
- **Email verification = first login.** When a registration's magic link is consumed for the first time, set `users.confirmed_at = now()`. This replaces a separate "confirm-email" flow.
- **Email enumeration** — registration with an existing email returns the same "check your email" page as a fresh registration, and re-sends a magic link to the existing account (no leak of "exists or not").
- **Mailgun real send is opt-in.** If `MAIL_NOOP=true` (default) the noop mailer is used. Set `MAIL_NOOP=false` plus `MAILGUN_API_KEY`, `MAILGUN_DOMAIN`, optional `MAILGUN_BASE_URL` to use Mailgun.
- **No SMTP fallback in P1c.** The interface is small enough that adding a `mail.SMTP` later is a one-task addition.

---

## File Structure

| Path | Purpose | Created in task |
|---|---|---|
| `go.mod`, `go.sum` | mailgun-go dep | T1 |
| `internal/config/config.go` | Add Mailgun env-var loading + validation | T1 |
| `internal/mail/mailgun.go` | `Mailgun` impl over `mailgun-go/v4` | T2 |
| `internal/mail/factory.go` | `NewFromConfig(*Config, *slog.Logger) Mailer` | T3 |
| `internal/store/queries/users.sql` | Append `MarkUserConfirmed :exec` | T4 |
| `internal/domain/accounts/repository.go` | `MarkConfirmed(ctx, userID)` method | T4 |
| `internal/site/handlers/auth.go` | `RegisterNew`, `RegisterCreate` handlers; LoginConfirm marks confirmed | T5 |
| `internal/site/templates/auth.go` | Add `RegisterPage`, `RegisterRequestedPage` templates | T5 |
| `internal/site/router.go` | Mount `/users/register` GET + POST | T5 |
| `internal/site/handlers/auth_test.go` | E2E for registration → magic-link → confirmed_at set | T6 |
| `cmd/acai/serve.go` | Use `mail.NewFromConfig` instead of always-noop | T7 |

---

## Task 1: Mailgun dep + Config

- [ ] **Step 1:** `go get github.com/mailgun/mailgun-go/v4 && go mod tidy`

- [ ] **Step 2:** Update `internal/config/config.go` — add fields:

```go
	// Mailgun configuration. Read only when MailNoop=false. The base URL
	// defaults to the Mailgun US region; EU users override to
	// "https://api.eu.mailgun.net/v3".
	MailgunAPIKey  string
	MailgunDomain  string
	MailgunBaseURL string
```

In `Load()`, after the existing MAIL block:

```go
	cfg.MailgunAPIKey = os.Getenv("MAILGUN_API_KEY")
	cfg.MailgunDomain = os.Getenv("MAILGUN_DOMAIN")
	cfg.MailgunBaseURL = getenvDefault("MAILGUN_BASE_URL", "https://api.mailgun.net/v3")

	if !cfg.MailNoop {
		if cfg.MailgunAPIKey == "" {
			return nil, errors.New("config: MAILGUN_API_KEY required when MAIL_NOOP=false")
		}
		if cfg.MailgunDomain == "" {
			return nil, errors.New("config: MAILGUN_DOMAIN required when MAIL_NOOP=false")
		}
	}
```

(Add `import "errors"` if not present.)

- [ ] **Step 3:** Tests in `internal/config/config_test.go`:

```go
func TestLoad_MailgunRequiredWhenMailNoopFalse(t *testing.T) {
	t.Setenv("MAIL_NOOP", "false")
	t.Setenv("MAILGUN_API_KEY", "")
	t.Setenv("MAILGUN_DOMAIN", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load with MAIL_NOOP=false and no Mailgun creds should error")
	}
}

func TestLoad_MailgunDefaults(t *testing.T) {
	t.Setenv("MAIL_NOOP", "true")
	t.Setenv("MAILGUN_BASE_URL", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MailgunBaseURL != "https://api.mailgun.net/v3" {
		t.Errorf("MailgunBaseURL = %q, want US default", cfg.MailgunBaseURL)
	}
}

func TestLoad_MailgunNoopBypassesValidation(t *testing.T) {
	t.Setenv("MAIL_NOOP", "true")
	t.Setenv("MAILGUN_API_KEY", "")
	t.Setenv("MAILGUN_DOMAIN", "")

	if _, err := config.Load(); err != nil {
		t.Fatalf("Load with MAIL_NOOP=true should not require Mailgun creds: %v", err)
	}
}
```

- [ ] **Step 4:** `just precommit` exits 0.

- [ ] **Step 5:** Commit: `feat(p1c): add Mailgun config (MAILGUN_API_KEY, MAILGUN_DOMAIN, MAILGUN_BASE_URL)`

---

## Task 2: `mail.Mailgun` implementation

- [ ] **Step 1:** Create `internal/mail/mailgun.go`:

```go
package mail

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mailgun/mailgun-go/v4"
)

// Mailgun sends transactional email via Mailgun's HTTP API.
type Mailgun struct {
	mg     mailgun.Mailgun
	log    *slog.Logger
	domain string
}

// NewMailgun returns a *Mailgun wired to the given API credentials.
// baseURL selects the region (US default; EU users pass
// "https://api.eu.mailgun.net/v3").
func NewMailgun(domain, apiKey, baseURL string, log *slog.Logger) *Mailgun {
	mg := mailgun.NewMailgun(domain, apiKey)
	if baseURL != "" {
		mg.SetAPIBase(baseURL)
	}
	return &Mailgun{mg: mg, log: log, domain: domain}
}

// SendMagicLink sends a magic-link email via Mailgun.
func (m *Mailgun) SendMagicLink(ctx context.Context, args MagicLinkArgs) error {
	from := fmt.Sprintf("%s <%s>", args.FromName, args.FromEmail)
	subject := "Your sign-in link"
	plain := fmt.Sprintf(
		"Hi,\n\nClick the link below to sign in:\n\n%s\n\nThis link expires in 15 minutes. If you didn't request it, ignore this message.\n",
		args.URL,
	)
	html := fmt.Sprintf(
		`<p>Hi,</p><p><a href="%s">Click here to sign in</a> (link expires in 15 minutes).</p><p>If you didn't request this, ignore this message.</p>`,
		args.URL,
	)

	msg := m.mg.NewMessage(from, subject, plain, args.To)
	msg.SetHtml(html)

	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, id, err := m.mg.Send(sendCtx, msg)
	if err != nil {
		m.log.Warn("mail.mailgun: send failed", "to", args.To, "error", err)
		return fmt.Errorf("mail.mailgun: send: %w", err)
	}
	m.log.Info("mail.mailgun: sent", "to", args.To, "id", id, "response", resp)
	return nil
}
```

- [ ] **Step 2:** Quick smoke test that the constructor produces a usable struct (no actual send — that requires real creds). Create `internal/mail/mailgun_test.go`:

```go
package mail_test

import (
	"io"
	"log/slog"
	"testing"

	"github.com/acai-sh/server/internal/mail"
)

func TestNewMailgun_ProducesUsableStruct(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	m := mail.NewMailgun("example.com", "test-key", "https://api.mailgun.net/v3", logger)
	if m == nil {
		t.Fatal("NewMailgun returned nil")
	}
}

func TestNewMailgun_EmptyBaseURLLeavesDefault(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	m := mail.NewMailgun("example.com", "test-key", "", logger)
	if m == nil {
		t.Fatal("NewMailgun returned nil")
	}
}
```

(Module path is `github.com/jadams-positron/acai-sh-server` — adjust the import accordingly when implementing.)

- [ ] **Step 3:** `just precommit` exits 0.

- [ ] **Step 4:** Commit: `feat(p1c): add mail.Mailgun implementation over mailgun-go/v4`

---

## Task 3: Mailer factory

- [ ] **Step 1:** Create `internal/mail/factory.go`:

```go
package mail

import (
	"log/slog"

	"github.com/jadams-positron/acai-sh-server/internal/config"
)

// NewFromConfig returns the Mailer implementation indicated by cfg. If
// cfg.MailNoop is true, returns *Noop; otherwise returns *Mailgun.
//
// Caller is responsible for cfg validation — config.Load will already have
// errored if MAIL_NOOP=false and MAILGUN_API_KEY/DOMAIN are missing.
func NewFromConfig(cfg *config.Config, log *slog.Logger) Mailer {
	if cfg.MailNoop {
		return NewNoop(log)
	}
	return NewMailgun(cfg.MailgunDomain, cfg.MailgunAPIKey, cfg.MailgunBaseURL, log)
}
```

- [ ] **Step 2:** Test in `internal/mail/factory_test.go`:

```go
package mail_test

import (
	"io"
	"log/slog"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/config"
	"github.com/jadams-positron/acai-sh-server/internal/mail"
)

func TestNewFromConfig_NoopWhenMailNoopTrue(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	cfg := &config.Config{MailNoop: true}

	m := mail.NewFromConfig(cfg, logger)
	if _, ok := m.(*mail.Noop); !ok {
		t.Errorf("expected *mail.Noop, got %T", m)
	}
}

func TestNewFromConfig_MailgunWhenMailNoopFalse(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	cfg := &config.Config{
		MailNoop:       false,
		MailgunDomain:  "example.com",
		MailgunAPIKey:  "test-key",
		MailgunBaseURL: "https://api.mailgun.net/v3",
	}

	m := mail.NewFromConfig(cfg, logger)
	if _, ok := m.(*mail.Mailgun); !ok {
		t.Errorf("expected *mail.Mailgun, got %T", m)
	}
}
```

- [ ] **Step 3:** Commit: `feat(p1c): add mail.NewFromConfig factory`

---

## Task 4: `MarkUserConfirmed` query + repo method

- [ ] **Step 1:** Append to `internal/store/queries/users.sql`:

```sql
-- name: MarkUserConfirmed :exec
UPDATE users
SET confirmed_at = ?, updated_at = ?
WHERE id = ? AND confirmed_at IS NULL;
```

(The `AND confirmed_at IS NULL` ensures we don't overwrite a previous confirmation timestamp — first-confirm-wins semantics.)

- [ ] **Step 2:** Run `just gen` to regenerate sqlc.

- [ ] **Step 3:** Add to `internal/domain/accounts/repository.go`:

```go
// MarkConfirmed sets users.confirmed_at to now() iff it was previously NULL.
// Idempotent: re-confirming an already-confirmed user is a no-op (the
// migration uses `WHERE confirmed_at IS NULL`).
func (r *Repository) MarkConfirmed(ctx context.Context, userID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	q := sqlc.New(r.db.Write)
	if err := q.MarkUserConfirmed(ctx, sqlc.MarkUserConfirmedParams{
		ConfirmedAt: &now,
		UpdatedAt:   now,
		ID:          userID,
	}); err != nil {
		return fmt.Errorf("accounts: MarkConfirmed: %w", err)
	}
	return nil
}
```

(`ConfirmedAt` is `*string` because the column is nullable and `emit_pointers_for_null_types: true` is on. Adjust per the actual generated type.)

- [ ] **Step 4:** Test in `internal/domain/accounts/repository_test.go`:

```go
func TestRepository_MarkConfirmed_SetsTimestampOnce(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	u, err := repo.CreateUser(ctx, accounts.CreateUserParams{
		Email:          "carol@example.com",
		HashedPassword: "",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ConfirmedAt != nil {
		t.Errorf("expected new user ConfirmedAt = nil, got %v", u.ConfirmedAt)
	}

	if err := repo.MarkConfirmed(ctx, u.ID); err != nil {
		t.Fatalf("MarkConfirmed: %v", err)
	}

	got, err := repo.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.ConfirmedAt == nil {
		t.Errorf("expected ConfirmedAt to be non-nil after MarkConfirmed")
	}

	first := *got.ConfirmedAt
	time.Sleep(50 * time.Millisecond)
	if err := repo.MarkConfirmed(ctx, u.ID); err != nil {
		t.Fatalf("second MarkConfirmed: %v", err)
	}
	again, _ := repo.GetUserByID(ctx, u.ID)
	if !again.ConfirmedAt.Equal(first) {
		t.Errorf("re-confirm changed timestamp: was %v, now %v (should be no-op)", first, *again.ConfirmedAt)
	}
}
```

- [ ] **Step 5:** `just precommit` exits 0.

- [ ] **Step 6:** Commit: `feat(p1c): add accounts.Repository.MarkConfirmed`

---

## Task 5: Registration handlers + page + route

- [ ] **Step 1:** Append to `internal/site/templates/auth.go`:

```go
// RegisterPage renders the email-only registration form.
var RegisterPage = template.Must(template.New("register").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Sign up — Acai</title>
</head>
<body>
  <h1>Sign up</h1>
  <p>We'll send you a magic-link to log in. No password needed.</p>
  {{if .Flash}}<p style="color: #b00">{{.Flash}}</p>{{end}}
  <form method="post" action="/users/register">
    <input type="hidden" name="{{.CSRFFieldName}}" value="{{.CSRFToken}}">
    <label>Email <input type="email" name="email" required autofocus></label>
    <button type="submit">Sign up</button>
  </form>
  <p>Already have an account? <a href="/users/log-in">Log in</a>.</p>
</body>
</html>`))

// RegisterPageData is the input for RegisterPage.
type RegisterPageData struct {
	Flash         string
	CSRFFieldName string
	CSRFToken     string
}

// RegisterRequestedPage renders the post-submit confirmation.
var RegisterRequestedPage = template.Must(template.New("register_requested").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Check your email — Acai</title>
</head>
<body>
  <h1>Check your email</h1>
  <p>We've sent a magic-link to <strong>{{.Email}}</strong>. Click it to log in.</p>
</body>
</html>`))

// RegisterRequestedPageData is the input for RegisterRequestedPage.
type RegisterRequestedPageData struct {
	Email string
}
```

- [ ] **Step 2:** Add `RegisterNew` and `RegisterCreate` to `internal/site/handlers/auth.go`:

```go
// RegisterNew GETs the sign-up form.
func RegisterNew(_ *AuthDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.RegisterPage.Execute(w, templates.RegisterPageData{
			CSRFFieldName: "gorilla.csrf.Token",
			CSRFToken:     csrf.Token(r),
		})
	}
}

// RegisterCreate POSTs the form: creates a new user (with NULL password) if
// not already present, then sends a magic-link. Always renders the same
// success page regardless of whether the email already existed (no enumeration).
func RegisterCreate(d *AuthDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		email := r.PostForm.Get("email")
		if email == "" {
			renderRegisterWithFlash(w, r, "Email is required.")
			return
		}

		user, err := d.Accounts.GetUserByEmail(r.Context(), email)
		switch {
		case err == nil:
			// already exists — re-send a login link to that account
		case accounts.IsNotFound(err):
			user, err = d.Accounts.CreateUser(r.Context(), accounts.CreateUserParams{
				Email:          email,
				HashedPassword: "",
			})
			if err != nil {
				d.Logger.Error("register: create user", "error", err)
				// Still render success to avoid leaking the failure mode.
			}
		default:
			d.Logger.Error("register: lookup user", "error", err)
		}

		if user != nil {
			url, _, genErr := d.MagicLink.GenerateLoginURL(r.Context(), user)
			if genErr == nil {
				_ = d.Mailer.SendMagicLink(r.Context(), mail.MagicLinkArgs{
					To:        user.Email,
					URL:       url,
					FromEmail: d.FromEmail,
					FromName:  d.FromName,
				})
			} else {
				d.Logger.Warn("register: generate magic link", "error", genErr)
			}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.RegisterRequestedPage.Execute(w, templates.RegisterRequestedPageData{Email: email})
	}
}

func renderRegisterWithFlash(w http.ResponseWriter, r *http.Request, flash string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = templates.RegisterPage.Execute(w, templates.RegisterPageData{
		Flash:         flash,
		CSRFFieldName: "gorilla.csrf.Token",
		CSRFToken:     csrf.Token(r),
	})
}
```

- [ ] **Step 3:** Update `LoginConfirm` to call `MarkConfirmed` after successful token consume:

```go
// LoginConfirm consumes a magic-link token, installs the session, marks the
// user confirmed (idempotent), then redirects to /teams.
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

		// First-login confirmation. No-op if already confirmed.
		if err := d.Accounts.MarkConfirmed(r.Context(), user.ID); err != nil {
			d.Logger.Warn("login: mark confirmed", "error", err)
			// non-fatal — proceed with login
		}

		if err := d.Sessions.RenewToken(r.Context()); err != nil {
			d.Logger.Warn("login: renew token", "error", err)
		}

		auth.Login(r.Context(), d.Sessions, user.ID)
		http.Redirect(w, r, "/teams", http.StatusSeeOther)
	}
}
```

- [ ] **Step 4:** Update `internal/site/router.go::MountAuthRoutes` to mount `/users/register`:

```go
	// Routes for unauthenticated users only.
	r.Group(func(r chi.Router) {
		r.Use(csrfMiddleware)
		r.Use(auth.RedirectIfAuth)
		r.Get("/users/log-in", handlers.LoginNew(deps))
		r.Post("/users/log-in", handlers.LoginCreate(deps))
		r.Get("/users/register", handlers.RegisterNew(deps))
		r.Post("/users/register", handlers.RegisterCreate(deps))
	})
```

- [ ] **Step 5:** `just precommit` exits 0.

- [ ] **Step 6:** Commit: `feat(p1c): add /users/register handler and page; mark user confirmed on first login`

---

## Task 6: E2E test for registration → magic-link → confirmed

- [ ] **Step 1:** Append to `internal/site/handlers/auth_test.go`:

```go
func TestRegister_FullFlowMarksUserConfirmed(t *testing.T) {
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
	mailer := &captureMailer{}
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
		DB: db, Sessions: sessionManager, Accounts: repo, AuthHandlerDeps: authDeps,
		CSRFKey: []byte(cfg.SecretKeyBase[:32]), SecureCookie: false, Version: "test",
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	ts := httptest.NewServer(plaintextWrapper(srv.Handler()))
	defer ts.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// 1. GET /users/register to obtain CSRF.
	req1, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/users/register", http.NoBody)
	resp, err := client.Do(req1)
	if err != nil {
		t.Fatalf("GET /users/register: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	csrfToken := html.UnescapeString(extractCSRFToken(string(body)))
	if csrfToken == "" {
		t.Fatalf("CSRF token not found in register form; body=%s", body)
	}

	// 2. POST /users/register with new email.
	form := url.Values{
		"email":              {"newcomer@example.com"},
		"gorilla.csrf.Token": {csrfToken},
	}
	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/users/register", strings.NewReader(form.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = client.Do(req2)
	if err != nil {
		t.Fatalf("POST /users/register: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register POST status = %d, want 200", resp.StatusCode)
	}
	if mailer.url == "" {
		t.Fatal("mailer did not receive a magic-link URL")
	}

	// 3. User now exists in DB but is unconfirmed.
	created, err := repo.GetUserByEmail(context.Background(), "newcomer@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail (newcomer): %v", err)
	}
	if created.ConfirmedAt != nil {
		t.Errorf("new user already confirmed: %v", created.ConfirmedAt)
	}

	// 4. Click the magic-link.
	confirmPath := strings.TrimPrefix(mailer.url, "http://localhost")
	req3, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+confirmPath, http.NoBody)
	resp, err = client.Do(req3)
	if err != nil {
		t.Fatalf("GET confirm: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("confirm status = %d, want 303", resp.StatusCode)
	}

	// 5. User is now confirmed.
	confirmed, err := repo.GetUserByEmail(context.Background(), "newcomer@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail after confirm: %v", err)
	}
	if confirmed.ConfirmedAt == nil {
		t.Errorf("user not marked confirmed after first login")
	}

	// 6. /teams shows the user.
	req4, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/teams", http.NoBody)
	resp, err = client.Do(req4)
	if err != nil {
		t.Fatalf("GET /teams: %v", err)
	}
	tbody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/teams status = %d; body=%s", resp.StatusCode, tbody)
	}
	if !strings.Contains(string(tbody), "newcomer@example.com") {
		t.Errorf("/teams body did not include user email; body=%s", tbody)
	}
}
```

(Reuses `plaintextWrapper`, `captureMailer`, `extractCSRFToken` helpers introduced by the existing `TestLogin_FullMagicLinkFlow` in the same file. Add the `html` import: `"html"`.)

- [ ] **Step 2:** Run: `go test ./internal/site/handlers/... -v -run TestRegister`

- [ ] **Step 3:** Commit: `test(p1c): e2e registration flow with confirmed_at side-effect`

---

## Task 7: Wire the factory in `cmd/acai/serve.go`

- [ ] **Step 1:** Edit `cmd/acai/serve.go` — replace the always-noop with `mail.NewFromConfig`:

```go
mailer := mail.NewFromConfig(cfg, logger)
```

(Drop the `if !cfg.MailNoop` warning printf — the factory handles selection cleanly. If the operator wanted Mailgun but had bad creds, `config.Load` already errored out at boot.)

- [ ] **Step 2:** Build smoke:

```bash
just build
DATABASE_PATH=/tmp/acai-p1c.db HTTP_PORT=4002 \
  SECRET_KEY_BASE="$(openssl rand -hex 32 2>/dev/null)" \
  MAIL_NOOP=true \
  ./acai serve &
SERVER_PID=$!
sleep 1
curl -fsS http://localhost:4002/_health | jq .
kill -INT $SERVER_PID
wait $SERVER_PID 2>/dev/null
rm -f /tmp/acai-p1c.db /tmp/acai-p1c.db-shm /tmp/acai-p1c.db-wal
just clean
```

- [ ] **Step 3:** Commit: `feat(p1c): wire mail.NewFromConfig in serve subcommand`

---

## Task 8: Push and verify CI

- [ ] **Step 1:** `git push` and watch the new run on `jadams-positron/acai-sh-server` until conclusion.

- [ ] **Step 2:** If failure, capture the failing log and escalate.

- [ ] **Step 3:** Mark P1c done. P1d (templ + Tailwind v4 + Datastar runtime) gets its own plan.

---

## Self-Review

- **Spec coverage:** Registration ✓ (was missing). Production mailer ✓ (Mailgun via mailgun-go/v4 per spec §2 stack picks). Mailer interface unchanged — single new impl.
- **No-enumeration:** Both `/users/log-in` and `/users/register` POSTs return identical responses regardless of whether the email exists. Registration "creates if missing, sends link otherwise."
- **Email-confirmation = first-login:** No separate `/users/confirm-email/:token` route in P1c. The login-confirm path also marks `confirmed_at` if it was NULL. (A separate change-email confirmation flow remains unbuilt; lands when account-settings UI does, P3+.)
- **Tests cover:** mailer factory, config validation, MarkConfirmed idempotency, end-to-end registration flow including the confirmed_at side-effect.
- **Type consistency:** `Mailer` interface unchanged. `*mail.Mailgun` satisfies `Mailer`. Factory returns `Mailer` (interface).

## Risk register

- **mailgun-go/v4 API churn** — The `NewMessage`/`SetHtml`/`Send` API in v4 is stable, but `mailgun.Mailgun` is an interface with a `*mailgun.MailgunImpl` default. If the constructor signature changed, adjust accordingly.
- **Mailgun region default** — US default. EU users must explicitly set `MAILGUN_BASE_URL=https://api.eu.mailgun.net/v3`. Documented in the env-var migration table later in the spec.
- **`time.Time.Equal` for confirmed-at idempotency** — comparing parsed timestamps must use `.Equal`, not `==`. Test uses `.Equal`.
