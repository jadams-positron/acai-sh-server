package handlers_test

import (
	"context"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/csrf"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/config"
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/mail"
	"github.com/jadams-positron/acai-sh-server/internal/ops"
	"github.com/jadams-positron/acai-sh-server/internal/server"
	"github.com/jadams-positron/acai-sh-server/internal/site/handlers"
	"github.com/jadams-positron/acai-sh-server/internal/store"
)

// captureMailer captures the magic-link URL instead of sending.
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
	if _, err := repo.CreateUser(context.Background(), accounts.CreateUserParams{
		Email:          "alice@example.com",
		HashedPassword: "$argon2id$dummy",
	}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

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
		DB:              db,
		Sessions:        sessionManager,
		Accounts:        repo,
		AuthHandlerDeps: authDeps,
		CSRFKey:         []byte(cfg.SecretKeyBase[:32]),
		SecureCookie:    false,
		Version:         "test",
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	// Wrap the handler so gorilla/csrf knows this is a plain-HTTP test server,
	// which skips the TLS-only Referer/Origin enforcement.
	plaintextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.Handler().ServeHTTP(w, csrf.PlaintextHTTPRequest(r))
	})
	ts := httptest.NewServer(plaintextHandler)
	defer ts.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Step 1: GET /users/log-in to obtain CSRF token + cookie.
	req1, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/users/log-in", http.NoBody)
	resp, err := client.Do(req1)
	if err != nil {
		t.Fatalf("GET /users/log-in: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	csrfToken := extractCSRFToken(string(body))
	if csrfToken == "" {
		t.Fatalf("CSRF token not found; body=%s", body)
	}
	// html/template HTML-escapes attribute values; unescape before submitting.
	csrfToken = html.UnescapeString(csrfToken)

	// Step 2: POST /users/log-in.
	form := url.Values{
		"email":              {"alice@example.com"},
		"gorilla.csrf.Token": {csrfToken},
	}
	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/users/log-in", strings.NewReader(form.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = client.Do(req2)
	if err != nil {
		t.Fatalf("POST /users/log-in: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if mailer.url == "" {
		t.Fatal("mailer did not capture a magic-link URL")
	}

	// Step 3: GET the magic-link URL — rewrite host since mailer.url uses "http://localhost".
	confirmPath := strings.TrimPrefix(mailer.url, "http://localhost")
	req3, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+confirmPath, http.NoBody)
	resp, err = client.Do(req3)
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
	req4, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/teams", http.NoBody)
	resp, err = client.Do(req4)
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

func extractCSRFToken(body string) string {
	const needle = `name="gorilla.csrf.Token" value="`
	_, after, ok := strings.Cut(body, needle)
	if !ok {
		return ""
	}
	before, _, ok := strings.Cut(after, `"`)
	if !ok {
		return ""
	}
	return before
}
