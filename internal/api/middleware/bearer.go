// Package middleware holds API-pipeline HTTP middleware: bearer auth, size
// caps, rate limiting.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/jadams-positron/acai-sh-server/internal/api/apierror"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
)

type ctxKey struct{ name string }

var (
	tokenCtxKey = ctxKey{"api.token"}
	teamCtxKey  = ctxKey{"api.team"}
)

// TokenFrom returns the *teams.AccessToken attached to ctx, or nil.
func TokenFrom(ctx context.Context) *teams.AccessToken {
	t, _ := ctx.Value(tokenCtxKey).(*teams.AccessToken)
	return t
}

// TeamFrom returns the *teams.Team attached to ctx, or nil.
func TeamFrom(ctx context.Context) *teams.Team {
	t, _ := ctx.Value(teamCtxKey).(*teams.Team)
	return t
}

// BearerAuth reads the Authorization header, validates the bearer token via
// repo.VerifyAccessToken, and attaches *AccessToken + *Team to the request
// context. On any failure: 401 with the standard app-error envelope.
func BearerAuth(repo *teams.Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawHeader := r.Header.Get("Authorization")
			if rawHeader == "" {
				apierror.WriteAppError(w, http.StatusUnauthorized, "Authorization header required", "")
				return
			}
			const prefix = "Bearer "
			if !strings.HasPrefix(rawHeader, prefix) {
				apierror.WriteAppError(w, http.StatusUnauthorized, "Authorization header must use Bearer scheme", "")
				return
			}
			plaintext := strings.TrimSpace(strings.TrimPrefix(rawHeader, prefix))
			if plaintext == "" {
				apierror.WriteAppError(w, http.StatusUnauthorized, "Invalid or missing bearer token", "")
				return
			}

			token, team, err := repo.VerifyAccessToken(r.Context(), plaintext)
			if err != nil {
				apierror.WriteAppError(w, http.StatusUnauthorized, "Invalid or expired bearer token", "")
				return
			}

			ctx := context.WithValue(r.Context(), tokenCtxKey, token)
			ctx = context.WithValue(ctx, teamCtxKey, team)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
