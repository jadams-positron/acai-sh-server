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
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
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
		_ = views.LoginPage(views.LoginPageProps{CSRFToken: csrf.Token(r)}).Render(r.Context(), w)
	}
}

// LoginCreate POSTs the login form: looks up user by email, generates a
// magic-link, sends via mailer, then renders the "check your email" page.
// Always returns the same response regardless of email existence.
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
			// Don't leak; render success page.
		default:
			d.Logger.Warn("login: lookup user", "error", err)
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = views.LoginRequestedPage(views.LoginRequestedProps{Email: email}).Render(r.Context(), w)
	}
}

// LoginConfirm consumes a magic-link token, marks confirmed, installs session.
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

		if err := d.Accounts.MarkConfirmed(r.Context(), user.ID); err != nil {
			d.Logger.Warn("login: mark confirmed", "error", err)
		}

		if err := d.Sessions.RenewToken(r.Context()); err != nil {
			d.Logger.Warn("login: renew token", "error", err)
		}

		auth.Login(r.Context(), d.Sessions, user.ID)
		http.Redirect(w, r, "/teams", http.StatusSeeOther)
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

// RegisterNew GETs the sign-up form.
func RegisterNew(_ *AuthDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = views.RegisterPage(views.RegisterPageProps{CSRFToken: csrf.Token(r)}).Render(r.Context(), w)
	}
}

// RegisterCreate POSTs the form: creates a new user (with NULL password) if
// not already present, then sends a magic-link. Always renders success page.
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
			// already exists — re-send a login link
		case accounts.IsNotFound(err):
			user, err = d.Accounts.CreateUser(r.Context(), accounts.CreateUserParams{
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
		_ = views.RegisterRequestedPage(views.RegisterRequestedProps{Email: email}).Render(r.Context(), w)
	}
}

func renderLoginWithFlash(w http.ResponseWriter, r *http.Request, flash string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = views.LoginPage(views.LoginPageProps{
		Flash:     flash,
		CSRFToken: csrf.Token(r),
	}).Render(r.Context(), w)
}

func renderRegisterWithFlash(w http.ResponseWriter, r *http.Request, flash string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = views.RegisterPage(views.RegisterPageProps{
		Flash:     flash,
		CSRFToken: csrf.Token(r),
	}).Render(r.Context(), w)
}
