// Package handlers is the home of site (browser) HTTP handlers.
package handlers

import (
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/gorilla/csrf"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/mail"
	"github.com/jadams-positron/acai-sh-server/internal/site/templates"
)

// AuthDeps groups the dependencies the auth handlers need.
type AuthDeps struct {
	Logger    *slog.Logger
	Sessions  *scs.SessionManager
	Accounts  *accounts.Repository
	MagicLink *auth.MagicLinkService
	Mailer    mail.Mailer
	FromEmail string
	FromName  string
}

// LoginNew GETs the login form.
func LoginNew(_ *AuthDeps) http.HandlerFunc {
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
// Always returns the same response regardless of email existence (no enumeration).
func LoginCreate(d *AuthDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		email := r.PostForm.Get("email")

		user, err := d.Accounts.GetUserByEmail(r.Context(), email)
		switch {
		case err == nil:
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
		case accounts.IsNotFound(err):
			// Don't leak whether the email exists; render the success page.
		default:
			d.Logger.Warn("login: lookup user", "error", err)
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.LoginRequestedPage.Execute(w, templates.LoginRequestedPageData{Email: email})
	}
}

// LoginConfirm consumes a magic-link token from the URL path and installs the session.
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

// RegisterNew GETs the registration form.
func RegisterNew(_ *AuthDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.RegisterPage.Execute(w, templates.RegisterPageData{
			CSRFFieldName: "gorilla.csrf.Token",
			CSRFToken:     csrf.Token(r),
		})
	}
}

// RegisterCreate POSTs the registration form: creates the user if they don't
// exist, then sends a magic-link. If the email is already registered, re-sends
// a magic link silently. Always renders the same success page (no enumeration).
func RegisterCreate(d *AuthDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		email := r.PostForm.Get("email")

		user, err := d.Accounts.GetUserByEmail(r.Context(), email)
		switch {
		case accounts.IsNotFound(err):
			// New user — create account with no password.
			created, createErr := d.Accounts.CreateUser(r.Context(), accounts.CreateUserParams{
				Email:          email,
				HashedPassword: "",
			})
			if createErr != nil {
				d.Logger.Warn("register: create user", "error", createErr)
				renderRegisterWithFlash(w, r, "Something went wrong. Please try again.")
				return
			}
			user = created
		case err == nil:
			// Existing user — fall through to send magic link below.
		default:
			d.Logger.Warn("register: lookup user", "error", err)
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

// LogOut destroys the session.
func LogOut(d *AuthDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := auth.Logout(r.Context(), d.Sessions); err != nil {
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

func renderRegisterWithFlash(w http.ResponseWriter, r *http.Request, flash string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = templates.RegisterPage.Execute(w, templates.RegisterPageData{
		Flash:         flash,
		CSRFFieldName: "gorilla.csrf.Token",
		CSRFToken:     csrf.Token(r),
	})
}
