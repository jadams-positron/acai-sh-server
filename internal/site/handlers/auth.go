// Package handlers is the home of site (browser) HTTP handlers.
package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/auth/google"
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/mail"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// AuthDeps groups the dependencies the auth handlers need.
type AuthDeps struct {
	Logger    *slog.Logger
	Sessions  *auth.SessionStore
	Accounts  *accounts.Repository
	MagicLink *auth.MagicLinkService
	Mailer    mail.Mailer
	FromEmail string
	FromName  string

	// Google is set when GOOGLE_AUTH_CLIENT_ID/SECRET are configured. When
	// nil, the /auth/google/* routes are not mounted and the login page
	// hides the Google button.
	Google *google.Provider
}

// csrfTokenFromEcho returns the CSRF token that echo's CSRF middleware injected.
func csrfTokenFromEcho(c echo.Context) string {
	if tok, ok := c.Get("csrf").(string); ok {
		return tok
	}
	return ""
}

// LoginNew GETs the login form.
func LoginNew(d *AuthDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return views.LoginPage(views.LoginPageProps{
			CSRFToken:     csrfTokenFromEcho(c),
			GoogleEnabled: d.Google != nil,
		}).Render(c.Request().Context(), c.Response())
	}
}

// LoginCreate POSTs the login form: looks up user by email, generates a
// magic-link, sends via mailer, then renders the "check your email" page.
// Always returns the same response regardless of email existence.
func LoginCreate(d *AuthDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		email := c.FormValue("email")

		user, err := d.Accounts.GetUserByEmail(c.Request().Context(), email)
		switch {
		case err == nil:
			url, _, genErr := d.MagicLink.GenerateLoginURL(c.Request().Context(), user)
			if genErr == nil {
				_ = d.Mailer.SendMagicLink(c.Request().Context(), mail.MagicLinkArgs{
					To:        user.Email,
					URL:       url,
					FromEmail: d.FromEmail,
					FromName:  d.FromName,
				})
			} else {
				d.Logger.Warn("login: generate magic link", "error", genErr)
			}
		case accounts.IsNotFound(err):
			// Don't leak; render success page.
		default:
			d.Logger.Warn("login: lookup user", "error", err)
		}

		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return views.LoginRequestedPage(views.LoginRequestedProps{
			Email:     email,
			CSRFToken: csrfTokenFromEcho(c),
		}).Render(c.Request().Context(), c.Response())
	}
}

// LoginConfirm consumes a magic-link token, marks confirmed, installs session.
func LoginConfirm(d *AuthDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		token := c.Param("token")
		if token == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "missing token")
		}
		user, err := d.MagicLink.ConsumeLoginToken(c.Request().Context(), token)
		if err != nil {
			d.Logger.Info("login: consume token failed", "error", err)
			return renderLoginWithFlash(c, d, "That magic link is invalid or expired. Please request a new one.")
		}

		if err := d.Accounts.MarkConfirmed(c.Request().Context(), user.ID); err != nil {
			d.Logger.Warn("login: mark confirmed", "error", err)
		}

		if err := d.Sessions.Login(c, user.ID); err != nil {
			d.Logger.Warn("login: save session", "error", err)
		}
		return c.Redirect(http.StatusSeeOther, "/teams")
	}
}

// LogOut destroys the session.
func LogOut(d *AuthDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		_ = d.Sessions.Logout(c)
		return c.Redirect(http.StatusSeeOther, "/users/log-in")
	}
}

// googleStateCookieName is the short-lived cookie that carries the
// OAuth state, nonce, and return_to between /auth/google/login and
// /auth/google/callback. We use a cookie (HttpOnly, SameSite=Lax,
// Secure when HTTPS) rather than the main session because the login
// flow doesn't have a session yet.
const googleStateCookieName = "_acai_oauth_state"

// GoogleLogin redirects to Google. POSTed (so the magic-link form's
// "Sign in with Google" button can submit it via a same-origin form),
// CSRF-protected by the existing site middleware. Generates state and
// nonce, stores them in a short-lived cookie, then 303s to Google.
func GoogleLogin(d *AuthDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		if d.Google == nil {
			return echo.NewHTTPError(http.StatusNotFound, "google sign-in not configured")
		}
		state, err := google.RandString(32)
		if err != nil {
			d.Logger.Error("google login: rand state", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed")
		}
		nonce, err := google.RandString(32)
		if err != nil {
			d.Logger.Error("google login: rand nonce", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed")
		}
		returnTo := google.SafeReturnTo(c.QueryParam("return_to"), "/teams")
		// Secure is conditional on c.Scheme() rather than always-true so
		// that local HTTP dev works; gosec G124 flags this but production
		// is HTTPS where Secure flips on automatically.
		c.SetCookie(&http.Cookie{ //nolint:gosec // Secure intentionally tied to URL scheme
			Name:     googleStateCookieName,
			Value:    state + "|" + nonce + "|" + returnTo,
			Path:     "/",
			HttpOnly: true,
			Secure:   c.Scheme() == "https",
			SameSite: http.SameSiteLaxMode,
			MaxAge:   600, // 10 minutes — Google flow always finishes faster
		})
		return c.Redirect(http.StatusFound, d.Google.AuthCodeURL(state, nonce))
	}
}

// GoogleCallback consumes the auth-code redirect from Google. CSRF
// middleware MUST be skipped on this route — Google can't include our
// CSRF token. Defends in depth via the state cookie + the OIDC nonce.
func GoogleCallback(d *AuthDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		if d.Google == nil {
			return echo.NewHTTPError(http.StatusNotFound, "google sign-in not configured")
		}
		code := c.QueryParam("code")
		state := c.QueryParam("state")
		if code == "" || state == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "missing code or state")
		}

		cookie, err := c.Cookie(googleStateCookieName)
		if err != nil || cookie.Value == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "missing state cookie")
		}
		// Cookie is "state|nonce|return_to" — opaque token, untrusted; we
		// validate state matches and treat return_to via SafeReturnTo.
		parts := strings.SplitN(cookie.Value, "|", 3)
		if len(parts) != 3 {
			return echo.NewHTTPError(http.StatusBadRequest, "malformed state cookie")
		}
		expectedState, expectedNonce, returnTo := parts[0], parts[1], parts[2]
		if state != expectedState {
			return echo.NewHTTPError(http.StatusBadRequest, "state mismatch")
		}

		// Clear the state cookie immediately — single-use.
		c.SetCookie(&http.Cookie{ //nolint:gosec // Secure intentionally tied to URL scheme; clearing cookie
			Name:     googleStateCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   c.Scheme() == "https",
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})

		claims, err := d.Google.Exchange(c.Request().Context(), code, expectedNonce)
		if err != nil {
			d.Logger.Warn("google callback: exchange", "error", err)
			return echo.NewHTTPError(http.StatusBadRequest, "code exchange failed")
		}
		if err := d.Google.Authorize(claims); err != nil {
			d.Logger.Warn("google callback: not in allowlist", "email", claims.Email, "hd", claims.HD)
			return echo.NewHTTPError(http.StatusForbidden, "not allowed")
		}

		// Look up or create the user. Google has verified the email so
		// MarkConfirmed unconditionally.
		user, err := d.Accounts.GetUserByEmail(c.Request().Context(), claims.Email)
		switch {
		case err == nil:
			// existing user
		case accounts.IsNotFound(err):
			user, err = d.Accounts.CreateUser(c.Request().Context(), accounts.CreateUserParams{
				Email: claims.Email,
			})
			if err != nil {
				d.Logger.Error("google callback: create user", "error", err)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed")
			}
		default:
			d.Logger.Error("google callback: lookup user", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed")
		}
		if err := d.Accounts.MarkConfirmed(c.Request().Context(), user.ID); err != nil {
			d.Logger.Warn("google callback: mark confirmed", "error", err)
		}
		if err := d.Sessions.Login(c, user.ID); err != nil {
			d.Logger.Error("google callback: save session", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed")
		}
		return c.Redirect(http.StatusSeeOther, google.SafeReturnTo(returnTo, "/teams"))
	}
}

// RegisterNew GETs the sign-up form.
func RegisterNew(_ *AuthDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return views.RegisterPage(views.RegisterPageProps{
			CSRFToken: csrfTokenFromEcho(c),
		}).Render(c.Request().Context(), c.Response())
	}
}

// RegisterCreate POSTs the form: creates a new user (with NULL password) if
// not already present, then sends a magic-link. Always renders success page.
func RegisterCreate(d *AuthDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		email := c.FormValue("email")
		if email == "" {
			return renderRegisterWithFlash(c, d, "Email is required.")
		}

		user, err := d.Accounts.GetUserByEmail(c.Request().Context(), email)
		switch {
		case err == nil:
			// already exists — re-send a login link
		case accounts.IsNotFound(err):
			user, err = d.Accounts.CreateUser(c.Request().Context(), accounts.CreateUserParams{
				Email:          email,
				HashedPassword: "",
			})
			if err != nil {
				d.Logger.Error("register: create user", "error", err)
			}
		default:
			d.Logger.Error("register: lookup user", "error", err)
		}

		if user != nil {
			url, _, genErr := d.MagicLink.GenerateLoginURL(c.Request().Context(), user)
			if genErr == nil {
				_ = d.Mailer.SendMagicLink(c.Request().Context(), mail.MagicLinkArgs{
					To:        user.Email,
					URL:       url,
					FromEmail: d.FromEmail,
					FromName:  d.FromName,
				})
			} else {
				d.Logger.Warn("register: generate magic link", "error", genErr)
			}
		}

		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return views.RegisterRequestedPage(views.RegisterRequestedProps{Email: email}).Render(c.Request().Context(), c.Response())
	}
}

func renderLoginWithFlash(c echo.Context, d *AuthDeps, flash string) error {
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	return views.LoginPage(views.LoginPageProps{
		Flash:         flash,
		CSRFToken:     csrfTokenFromEcho(c),
		GoogleEnabled: d.Google != nil,
	}).Render(c.Request().Context(), c.Response())
}

func renderRegisterWithFlash(c echo.Context, _ *AuthDeps, flash string) error {
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	return views.RegisterPage(views.RegisterPageProps{
		Flash:     flash,
		CSRFToken: csrfTokenFromEcho(c),
	}).Render(c.Request().Context(), c.Response())
}
