// Package config loads runtime configuration from environment variables.
//
// Fields are populated by Load and alphabetically sorted within each section.
// P0 adds LogLevel; P1a adds DatabasePath and HTTPPort; P1b adds
// SecretKeyBase, Mail*, and URL* fields; P1c adds Mailgun* fields.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

// Config is the validated runtime configuration. All fields are populated by Load.
type Config struct {
	// DatabasePath is the path to the SQLite database file. Default: "./acai.db".
	DatabasePath string

	// HTTPPort is the TCP port the HTTP server listens on. Range [1, 65535]. Default: 4000.
	HTTPPort int

	// LogLevel is one of "debug", "info", "warn", "error". Default: "info".
	LogLevel string

	// MailFromEmail is the RFC 5321 envelope sender address for outbound email.
	MailFromEmail string

	// MailFromName is the human-readable sender name for outbound email.
	MailFromName string

	// MailNoop disables all outbound mail when true; log entries are emitted
	// instead. Parsed from MAIL_NOOP env var (true/false). Default: false.
	MailNoop bool

	// MailgunAPIKey is the Mailgun private API key used to authenticate requests.
	// Required when MailNoop is false.
	MailgunAPIKey string

	// MailgunBaseURL is the Mailgun API base URL. Defaults to
	// https://api.mailgun.net/v3; override to https://api.eu.mailgun.net/v3 for
	// EU-region domains.
	MailgunBaseURL string

	// MailgunDomain is the Mailgun sending domain (e.g. "mg.example.com").
	// Required when MailNoop is false.
	MailgunDomain string

	// SecretKeyBase is the HMAC key used to sign session cookies and CSRF
	// tokens. Must be at least 32 bytes. Never commit a real value; use the
	// provided dev default only for local development.
	SecretKeyBase string

	// URLHost is the public hostname (no scheme, no port) used when generating
	// absolute URLs (e.g. magic-link emails). Default: "localhost".
	URLHost string

	// URLScheme is the public URL scheme. Allowed values: "http", "https".
	// Default: "http".
	URLScheme string
}

// unsafeDevSecret is the default SecretKeyBase used in development only.
// It is intentionally published here; rotate before any deployment.
const unsafeDevSecret = "UNSAFE_dev_secret_do_not_use_in_production_padding_xyz" //nolint:gosec // intentional dev-only placeholder; documentation value, not a real credential

// Load reads configuration from environment variables and validates it.
// Returns an error if any value fails validation.
func Load() (*Config, error) {
	cfg := &Config{
		DatabasePath:  getenvDefault("DATABASE_PATH", "./acai.db"),
		LogLevel:      getenvDefault("LOG_LEVEL", "info"),
		MailFromEmail: getenvDefault("MAIL_FROM_EMAIL", ""),
		MailFromName:  getenvDefault("MAIL_FROM_NAME", ""),
		SecretKeyBase: getenvDefault("SECRET_KEY_BASE", unsafeDevSecret),
		URLHost:       getenvDefault("URL_HOST", "localhost"),
		URLScheme:     getenvDefault("URL_SCHEME", "http"),
	}

	// HTTP_PORT
	portStr := getenvDefault("HTTP_PORT", "4000")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("config: invalid HTTP_PORT %q: not an integer", portStr)
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("config: invalid HTTP_PORT %d: must be in range [1, 65535]", port)
	}
	cfg.HTTPPort = port

	// LOG_LEVEL
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
		// ok
	default:
		return nil, fmt.Errorf("config: invalid LOG_LEVEL %q (allowed: debug, info, warn, error)", cfg.LogLevel)
	}

	// MAIL_NOOP
	mailNoopStr := getenvDefault("MAIL_NOOP", "false")
	switch mailNoopStr {
	case "true":
		cfg.MailNoop = true
	case "false":
		cfg.MailNoop = false
	default:
		return nil, fmt.Errorf("config: invalid MAIL_NOOP %q (allowed: true, false)", mailNoopStr)
	}

	// MAILGUN_*
	cfg.MailgunAPIKey = getenvDefault("MAILGUN_API_KEY", "")
	cfg.MailgunDomain = getenvDefault("MAILGUN_DOMAIN", "")
	cfg.MailgunBaseURL = getenvDefault("MAILGUN_BASE_URL", "https://api.mailgun.net/v3")
	if !cfg.MailNoop {
		if cfg.MailgunAPIKey == "" {
			return nil, errors.New("config: MAILGUN_API_KEY is required when MAIL_NOOP is false")
		}
		if cfg.MailgunDomain == "" {
			return nil, errors.New("config: MAILGUN_DOMAIN is required when MAIL_NOOP is false")
		}
	}

	// SECRET_KEY_BASE: must be at least 32 bytes
	if len(cfg.SecretKeyBase) < 32 {
		return nil, fmt.Errorf("config: SECRET_KEY_BASE must be at least 32 bytes (got %d)", len(cfg.SecretKeyBase))
	}

	// URL_SCHEME
	switch cfg.URLScheme {
	case "http", "https":
		// ok
	default:
		return nil, fmt.Errorf("config: invalid URL_SCHEME %q (allowed: http, https)", cfg.URLScheme)
	}

	return cfg, nil
}

func getenvDefault(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
