package google_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/auth/google"
)

func TestAuthorize(t *testing.T) {
	t.Parallel()

	verified := func(email, hd string) *google.Claims {
		return &google.Claims{
			Subject:       "sub-" + email,
			Email:         email,
			EmailVerified: true,
			HD:            hd,
		}
	}

	tests := []struct {
		name    string
		cfg     google.Config
		claims  *google.Claims
		wantErr bool
	}{
		{
			name: "allowed domain match",
			cfg: google.Config{
				AllowedDomains: []string{"positron.ai"},
			},
			claims:  verified("alex@positron.ai", "positron.ai"),
			wantErr: false,
		},
		{
			name: "allowed domain match, no hd in claim",
			cfg: google.Config{
				AllowedDomains: []string{"positron.ai"},
			},
			claims:  verified("alex@positron.ai", ""),
			wantErr: false,
		},
		{
			name: "domain mismatch",
			cfg: google.Config{
				AllowedDomains: []string{"positron.ai"},
			},
			claims:  verified("alex@evil.example", ""),
			wantErr: true,
		},
		{
			name: "hd lies — email is positron but hd claims evil",
			cfg: google.Config{
				AllowedDomains: []string{"positron.ai"},
			},
			claims:  verified("alex@positron.ai", "evil.example"),
			wantErr: true,
		},
		{
			name: "hd lies the other way — email is evil but hd claims positron",
			cfg: google.Config{
				AllowedDomains: []string{"positron.ai"},
			},
			claims:  verified("alex@evil.example", "positron.ai"),
			wantErr: true,
		},
		{
			name: "case-insensitive domain match",
			cfg: google.Config{
				AllowedDomains: []string{"Positron.AI"},
			},
			claims:  verified("alex@positron.ai", "positron.ai"),
			wantErr: false,
		},
		{
			name: "unverified email rejects domain match",
			cfg: google.Config{
				AllowedDomains: []string{"positron.ai"},
			},
			claims: &google.Claims{
				Subject:       "sub",
				Email:         "alex@positron.ai",
				EmailVerified: false,
				HD:            "positron.ai",
			},
			wantErr: true,
		},
		{
			name: "explicit email allowlist match",
			cfg: google.Config{
				AllowedEmails: []string{"alex@example.com"},
			},
			claims:  verified("alex@example.com", ""),
			wantErr: false,
		},
		{
			name: "case-insensitive email match",
			cfg: google.Config{
				AllowedEmails: []string{"Alex@Example.com"},
			},
			claims:  verified("alex@example.com", ""),
			wantErr: false,
		},
		{
			name: "explicit subject allowlist match (no email needed)",
			cfg: google.Config{
				AllowedSubjects: []string{"sub-machine-account"},
			},
			claims: &google.Claims{
				Subject:       "sub-machine-account",
				EmailVerified: false,
			},
			wantErr: false,
		},
		{
			name: "nil claims",
			cfg: google.Config{
				AllowedDomains: []string{"positron.ai"},
			},
			claims:  nil,
			wantErr: true,
		},
		{
			name:    "no allowlists is unauthorized — never reachable from NewProvider",
			cfg:     google.Config{},
			claims:  verified("alex@positron.ai", "positron.ai"),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// We can't call NewProvider here (it does network OIDC discovery),
			// so construct a Provider via reflection-free indirection: this
			// requires Provider to expose its allowlist or Authorize to be a
			// pure function — Authorize already only reads cfg. Mirror what
			// NewProvider would build by populating a Provider via the
			// package-internal cfg field through an exported test helper.
			p := google.NewProviderForTest(tc.cfg)
			err := p.Authorize(tc.claims)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
			if tc.wantErr && err != nil && !errors.Is(err, google.ErrIdentityNotAllowed) {
				t.Errorf("expected ErrIdentityNotAllowed, got %v", err)
			}
		})
	}
}

func TestSafeReturnTo(t *testing.T) {
	t.Parallel()

	const fallback = "/"
	tests := []struct {
		raw  string
		want string
	}{
		{"", "/"},
		{"/teams", "/teams"},
		{"/t/acme/p/ledger", "/t/acme/p/ledger"},
		{"//evil.com/anywhere", "/"},       // protocol-relative
		{"/\\evil.com/anywhere", "/"},      // backslash variant
		{"https://evil.com/", "/"},         // absolute
		{"javascript:alert(1)", "/"},       // scheme
		{"teams", "/"},                     // relative-no-leading-slash
		{"/with?query=1", "/with?query=1"}, // query strings allowed
		{"/with#frag", "/with#frag"},       // fragments allowed
	}
	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			got := google.SafeReturnTo(tc.raw, fallback)
			if got != tc.want {
				t.Errorf("SafeReturnTo(%q, %q) = %q, want %q", tc.raw, fallback, got, tc.want)
			}
		})
	}
}

func TestNewProvider_Validation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     google.Config
		wantSub string // substring of error message
	}{
		{
			name:    "missing client id",
			cfg:     google.Config{ClientSecret: "x", RedirectURL: "http://localhost/cb", AllowedDomains: []string{"a"}},
			wantSub: "ClientID",
		},
		{
			name:    "missing client secret",
			cfg:     google.Config{ClientID: "x", RedirectURL: "http://localhost/cb", AllowedDomains: []string{"a"}},
			wantSub: "ClientSecret",
		},
		{
			name:    "no allowlists",
			cfg:     google.Config{ClientID: "x", ClientSecret: "y", RedirectURL: "http://localhost/cb"},
			wantSub: "AllowedDomains",
		},
		{
			name:    "bad redirect url",
			cfg:     google.Config{ClientID: "x", ClientSecret: "y", RedirectURL: "not-a-url", AllowedDomains: []string{"a"}},
			wantSub: "RedirectURL",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := google.NewProvider(t.Context(), tc.cfg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("expected error containing %q, got %q", tc.wantSub, err.Error())
			}
		})
	}
}
