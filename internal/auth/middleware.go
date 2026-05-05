package auth

import (
	"net/http"

	"github.com/alexedwards/scs/v2"

	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
)

// LoadScope reads user_id from the session, fetches the user, and stores a
// *Scope on the request context. Anonymous requests get an empty Scope.
func LoadScope(mgr *scs.SessionManager, repo *accounts.Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			scope := &Scope{}

			if userID := mgr.GetString(ctx, sessionKeyUserID); userID != "" {
				user, err := repo.GetUserByID(ctx, userID)
				if err == nil {
					scope.User = user
				} else if accounts.IsNotFound(err) {
					_ = mgr.Destroy(ctx)
				}
			}

			next.ServeHTTP(w, r.WithContext(WithScope(ctx, scope)))
		})
	}
}

// RequireAuth redirects unauthenticated requests to /users/log-in.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ScopeFrom(r.Context()).IsAuthenticated() {
			http.Redirect(w, r, "/users/log-in", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RedirectIfAuth redirects authenticated requests to /teams. Use on the login page.
func RedirectIfAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ScopeFrom(r.Context()).IsAuthenticated() {
			http.Redirect(w, r, "/teams", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}
