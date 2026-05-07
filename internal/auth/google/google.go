// Package google adds Google OAuth (OIDC) sign-in to the existing
// magic-link auth. Modeled after positron-ai/api.positron.ai/api-server's
// internal/auth — same library stack (coreos/go-oidc + golang.org/x/oauth2),
// same hosted-domain enforcement (post-callback hd ↔ email cross-check),
// same safeReturnTo open-redirect guard.
//
// Key difference: we already have a users table and an existing
// SessionStore that keys sessions by user_id. After a Google login is
// authorized, the handler upserts a domain user record and installs the
// existing session — Google becomes one path among magic-link to the
// same authenticated state. Magic-link continues to work unchanged.
package google

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// DefaultIssuer is Google's OIDC issuer URL.
const DefaultIssuer = "https://accounts.google.com"

// Config carries the inputs for NewProvider.
type Config struct {
	Issuer          string   // default: DefaultIssuer
	ClientID        string   // required
	ClientSecret    string   // required
	RedirectURL     string   // required, absolute URL of /auth/google/callback
	AllowedDomains  []string // at least one of these or AllowedEmails / AllowedSubjects required
	AllowedEmails   []string
	AllowedSubjects []string
	PostLoginURL    string // default: "/"
}

// Provider wraps the OIDC verifier + OAuth2 config plus the allowlist.
type Provider struct {
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
	cfg      Config
}

// NewProvider constructs a Provider after running OIDC discovery against
// cfg.Issuer. Validates that at least one allowlist is non-empty so a
// misconfigured deployment can't accidentally let arbitrary Google users
// in.
func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RedirectURL == "" {
		return nil, errors.New("google auth: ClientID, ClientSecret, and RedirectURL are required")
	}
	if len(cfg.AllowedDomains) == 0 && len(cfg.AllowedEmails) == 0 && len(cfg.AllowedSubjects) == 0 {
		return nil, errors.New("google auth: at least one of AllowedDomains, AllowedEmails, AllowedSubjects must be set")
	}
	redirectU, err := url.Parse(cfg.RedirectURL)
	if err != nil || redirectU.Scheme == "" || redirectU.Host == "" {
		return nil, fmt.Errorf("google auth: invalid RedirectURL %q", cfg.RedirectURL)
	}
	issuer := cfg.Issuer
	if issuer == "" {
		issuer = DefaultIssuer
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("google auth: oidc discovery: %w", err)
	}
	return &Provider{
		oauth: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		cfg:      cfg,
	}, nil
}

// Claims is the trimmed-down ID-token shape we care about.
type Claims struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	HD            string // Google's hosted-domain claim
}

// AuthCodeURL returns the URL to redirect the user to start the auth
// flow. State and nonce are passed in by the caller; the caller is
// responsible for stashing them in the session before redirect.
func (p *Provider) AuthCodeURL(state, nonce string) string {
	return p.oauth.AuthCodeURL(state, oidc.Nonce(nonce))
}

// Exchange runs the code exchange and verifies the resulting ID token's
// nonce. Returns the parsed claims on success.
func (p *Provider) Exchange(ctx context.Context, code, expectedNonce string) (*Claims, error) {
	tok, err := p.oauth.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("google auth: code exchange: %w", err)
	}
	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, errors.New("google auth: response did not include id_token")
	}
	idTok, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("google auth: id_token verification: %w", err)
	}
	if expectedNonce == "" || idTok.Nonce != expectedNonce {
		return nil, errors.New("google auth: nonce mismatch")
	}
	var raw struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		HD            string `json:"hd"`
	}
	if err := idTok.Claims(&raw); err != nil {
		return nil, fmt.Errorf("google auth: claim extraction: %w", err)
	}
	return &Claims{
		Subject:       idTok.Subject,
		Email:         raw.Email,
		EmailVerified: raw.EmailVerified,
		Name:          raw.Name,
		HD:            raw.HD,
	}, nil
}

// ErrIdentityNotAllowed is returned by Authorize when the claims don't
// match any of the configured allowlists.
var ErrIdentityNotAllowed = errors.New("google auth: identity not in allowlist")

// Authorize checks the claims against the configured allowlists. The
// hosted-domain (hd) check cross-verifies hd against the email's domain
// to defend against IdPs that inject a synthetic hd value.
//
// Empty allowlists were rejected at construction time, so an
// unconfigured Provider cannot reach this method.
func (p *Provider) Authorize(c *Claims) error {
	if c == nil {
		return ErrIdentityNotAllowed
	}
	if slices.Contains(p.cfg.AllowedSubjects, c.Subject) {
		return nil
	}
	if c.EmailVerified && c.Email != "" {
		for _, e := range p.cfg.AllowedEmails {
			if strings.EqualFold(e, c.Email) {
				return nil
			}
		}
	}
	if len(p.cfg.AllowedDomains) > 0 && c.EmailVerified && c.Email != "" {
		emailDomain := ""
		if at := strings.LastIndex(c.Email, "@"); at >= 0 {
			emailDomain = c.Email[at+1:]
		}
		// Cross-check hd against email domain. Without this, an IdP
		// (e.g. Auth0 with a custom rule) could inject hd to bypass.
		domain := emailDomain
		if c.HD != "" {
			if !strings.EqualFold(c.HD, emailDomain) {
				return ErrIdentityNotAllowed
			}
			domain = c.HD
		}
		if domain != "" {
			for _, d := range p.cfg.AllowedDomains {
				if strings.EqualFold(d, domain) {
					return nil
				}
			}
		}
	}
	return ErrIdentityNotAllowed
}

// PostLoginRedirect returns the post-login URL given the (possibly
// untrusted) return_to from the original login request. Falls back to
// cfg.PostLoginURL (or "/") when raw is unsafe or empty.
func (p *Provider) PostLoginRedirect(raw string) string {
	fallback := p.cfg.PostLoginURL
	if fallback == "" {
		fallback = "/"
	}
	return SafeReturnTo(raw, fallback)
}

// SafeReturnTo returns raw if it is a same-origin relative path: a single
// leading "/", no scheme, no host, and not a protocol-relative ("//...")
// or backslash-prefixed ("/\...") form that some browsers normalize to
// "//". Otherwise it returns fallback. This prevents an open redirect
// via the `return_to` parameter on the login flow.
//
// Copied from positron-ai/api.positron.ai/api-server with the
// behavior locked into a unit-tested function so all callers share
// the same hardening.
func SafeReturnTo(raw, fallback string) string {
	if raw == "" {
		return fallback
	}
	if raw[0] != '/' {
		return fallback
	}
	if len(raw) >= 2 && (raw[1] == '/' || raw[1] == '\\') {
		return fallback
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fallback
	}
	if u.Scheme != "" || u.Host != "" {
		return fallback
	}
	return raw
}

// RandString returns base64url-encoded n random bytes. Used by the
// caller to generate state and nonce values before redirecting to
// Google. Exposed so tests can stub the source if needed.
func RandString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// AuthorizationHeader extracts a bearer token if present. Currently
// unused — magic-link is the primary auth path and Google sessions go
// through the existing cookie store. Kept here as a hook for if we
// add API-token-style Google auth later.
func AuthorizationHeader(h http.Header) string {
	v := h.Get("Authorization")
	if !strings.HasPrefix(v, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(v, "Bearer ")
}
