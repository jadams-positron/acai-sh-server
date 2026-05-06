// Package middleware holds API-pipeline HTTP middleware: bearer auth, size
// caps, rate limiting.
package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/api/apierror"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
)

const (
	tokenKey = "api.token"
	teamKey  = "api.team"
)

// TokenFromEcho returns the *teams.AccessToken attached to c, or nil.
func TokenFromEcho(c echo.Context) *teams.AccessToken {
	t, _ := c.Get(tokenKey).(*teams.AccessToken)
	return t
}

// TeamFromEcho returns the *teams.Team attached to c, or nil.
func TeamFromEcho(c echo.Context) *teams.Team {
	t, _ := c.Get(teamKey).(*teams.Team)
	return t
}

// BearerAuth reads Authorization, validates the bearer token, attaches
// *AccessToken + *Team to the echo context.
func BearerAuth(repo *teams.Repository) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			rawHeader := c.Request().Header.Get("Authorization")
			if rawHeader == "" {
				return apierror.WriteAppErrorEcho(c, http.StatusUnauthorized, "Authorization header required", "")
			}
			const prefix = "Bearer "
			if !strings.HasPrefix(rawHeader, prefix) {
				return apierror.WriteAppErrorEcho(c, http.StatusUnauthorized, "Authorization header must use Bearer scheme", "")
			}
			plaintext := strings.TrimSpace(strings.TrimPrefix(rawHeader, prefix))
			if plaintext == "" {
				return apierror.WriteAppErrorEcho(c, http.StatusUnauthorized, "Invalid or missing bearer token", "")
			}

			token, team, err := repo.VerifyAccessToken(c.Request().Context(), plaintext)
			if err != nil {
				return apierror.WriteAppErrorEcho(c, http.StatusUnauthorized, "Invalid or expired bearer token", "")
			}

			c.Set(tokenKey, token)
			c.Set(teamKey, team)
			return next(c)
		}
	}
}
