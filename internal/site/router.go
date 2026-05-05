// Package site mounts browser-facing HTTP routes.
package site

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/site/handlers"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// MountAuthRoutes registers the login/logout routes on r. Caller is expected
// to have mounted sessionManager.LoadAndSave + auth.LoadScope at the parent.
//
// CSRF is applied on the auth subtree (everywhere except the magic-link
// confirm GET, where the token IS the auth proof).
func MountAuthRoutes(r chi.Router, deps *handlers.AuthDeps, csrfKey []byte, secureCookie bool) {
	csrfMiddleware := csrf.Protect(csrfKey,
		csrf.Secure(secureCookie),
		csrf.Path("/"),
		csrf.SameSite(csrf.SameSiteLaxMode),
	)

	// Routes for unauthenticated users only.
	r.Group(func(r chi.Router) {
		r.Use(csrfMiddleware)
		r.Use(auth.RedirectIfAuth)
		r.Get("/users/log-in", handlers.LoginNew(deps))
		r.Post("/users/log-in", handlers.LoginCreate(deps))
		r.Get("/users/register", handlers.RegisterNew(deps))
		r.Post("/users/register", handlers.RegisterCreate(deps))
	})

	// Magic-link consume — bypasses CSRF.
	r.Get("/users/log-in/{token}", handlers.LoginConfirm(deps))

	// Logout always allowed.
	r.Group(func(r chi.Router) {
		r.Use(csrfMiddleware)
		r.Post("/users/log-out", handlers.LogOut(deps))
	})
}

// MountAuthRequiredStub mounts /teams as a P1d proof-of-life endpoint. /teams
// is wrapped with CSRF middleware so the embedded logout form's token is
// valid; the route still requires auth.
func MountAuthRequiredStub(r chi.Router, csrfKey []byte, secureCookie bool) {
	csrfMiddleware := csrf.Protect(csrfKey,
		csrf.Secure(secureCookie),
		csrf.Path("/"),
		csrf.SameSite(csrf.SameSiteLaxMode),
	)
	r.Group(func(r chi.Router) {
		r.Use(csrfMiddleware)
		r.Use(auth.RequireAuth)
		r.Get("/teams", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			s := auth.ScopeFrom(r.Context())
			_ = views.TeamsStub(views.TeamsStubProps{
				UserEmail: s.User.Email,
				CSRFToken: csrf.Token(r),
			}).Render(r.Context(), w)
		})
	})
}
