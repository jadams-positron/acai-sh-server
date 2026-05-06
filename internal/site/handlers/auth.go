// Package handlers is the home of site (browser) HTTP handlers.
package handlers

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
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
}

// csrfTokenFromEcho returns the CSRF token that echo's CSRF middleware injected.
func csrfTokenFromEcho(c echo.Context) string {
	if tok, ok := c.Get("csrf").(string); ok {
		return tok
	}
	return ""
}

// LoginNew GETs the login form.
func LoginNew(_ *AuthDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return views.LoginPage(views.LoginPageProps{
			CSRFToken: csrfTokenFromEcho(c),
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
		return views.LoginRequestedPage(views.LoginRequestedProps{Email: email}).Render(c.Request().Context(), c.Response())
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
		Flash:     flash,
		CSRFToken: csrfTokenFromEcho(c),
	}).Render(c.Request().Context(), c.Response())
}

func renderRegisterWithFlash(c echo.Context, d *AuthDeps, flash string) error {
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	return views.RegisterPage(views.RegisterPageProps{
		Flash:     flash,
		CSRFToken: csrfTokenFromEcho(c),
	}).Render(c.Request().Context(), c.Response())
}
