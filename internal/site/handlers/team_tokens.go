package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// TeamTokensDeps groups dependencies for the team tokens page.
type TeamTokensDeps struct {
	Logger *slog.Logger
	Teams  *teams.Repository
}

// loadTeamAsMember is a helper that loads the team by name and checks that the
// current user is a member. Returns (team, error). On hard failure it writes
// the appropriate HTTP error to c and returns a non-nil err.
func loadTeamAsMember(c echo.Context, repo *teams.Repository, logger *slog.Logger) (*teams.Team, error) {
	scope := auth.ScopeFromEcho(c)
	if scope.User == nil {
		if err := c.Redirect(http.StatusSeeOther, "/users/log-in"); err != nil {
			return nil, err
		}
		return nil, echo.ErrUnauthorized
	}
	teamName := c.Param("team_name")
	team, err := repo.GetByName(c.Request().Context(), teamName)
	if err != nil {
		if teams.IsNotFound(err) {
			return nil, echo.NewHTTPError(http.StatusNotFound, "team not found")
		}
		logger.Error("team tokens: GetByName", "error", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "failed to load team")
	}
	member, err := repo.IsMember(c.Request().Context(), team.ID, scope.User.ID)
	if err != nil {
		logger.Error("team tokens: IsMember", "error", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "failed to verify membership")
	}
	if !member {
		return nil, echo.NewHTTPError(http.StatusNotFound, "team not found")
	}
	return team, nil
}

// renderTeamTokens is the shared render helper for the tokens page.
func renderTeamTokens(c echo.Context, d *TeamTokensDeps, team *teams.Team, flash, flashType, newToken string) error {
	toks, err := d.Teams.ListAccessTokensForTeam(c.Request().Context(), team.ID)
	if err != nil {
		d.Logger.Error("team tokens: ListAccessTokensForTeam", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load tokens")
	}
	shell, err := buildShellChrome(c, d.Teams, team.Name+" · Tokens", team, "tokens", []views.Crumb{
		{Label: "Teams", HRef: "/teams"},
		{Label: team.Name, HRef: "/t/" + team.Name},
		{Label: "Tokens"},
	})
	if err != nil {
		d.Logger.Error("team tokens: shell chrome", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load tokens")
	}
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	return views.TeamTokens(views.TeamTokensProps{
		Shell:             shell,
		Team:              team,
		Tokens:            toks,
		CSRFToken:         csrfTokenFromEcho(c),
		Flash:             flash,
		FlashType:         flashType,
		NewlyCreatedToken: newToken,
	}).Render(c.Request().Context(), c.Response())
}

// TeamTokens renders GET /t/:team_name/tokens.
func TeamTokens(d *TeamTokensDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := auth.ScopeFromEcho(c)
		if scope.User == nil {
			return c.Redirect(http.StatusSeeOther, "/users/log-in")
		}
		team, err := loadTeamAsMember(c, d.Teams, d.Logger)
		if err != nil {
			return err
		}
		return renderTeamTokens(c, d, team, "", "", "")
	}
}

// TeamTokensCreate handles POST /t/:team_name/tokens.
func TeamTokensCreate(d *TeamTokensDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := auth.ScopeFromEcho(c)
		if scope.User == nil {
			return c.Redirect(http.StatusSeeOther, "/users/log-in")
		}
		team, err := loadTeamAsMember(c, d.Teams, d.Logger)
		if err != nil {
			return err
		}

		name := c.FormValue("name")
		if name == "" {
			c.Response().WriteHeader(http.StatusUnprocessableEntity)
			return renderTeamTokens(c, d, team, "Token name is required.", "error", "")
		}

		var expiresAt *time.Time
		if daysStr := c.FormValue("expires_days"); daysStr != "" {
			days, err := strconv.Atoi(daysStr)
			if err != nil || days < 1 {
				c.Response().WriteHeader(http.StatusUnprocessableEntity)
				return renderTeamTokens(c, d, team, "Expiry must be a positive number of days.", "error", "")
			}
			exp := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
			expiresAt = &exp
		}

		plaintext, err := d.Teams.CreateAccessToken(c.Request().Context(), teams.CreateAccessTokenParams{
			UserID:    scope.User.ID,
			TeamID:    team.ID,
			Name:      name,
			Scopes:    []string{},
			ExpiresAt: expiresAt,
		})
		if err != nil {
			d.Logger.Error("team tokens: CreateAccessToken", "error", err)
			c.Response().WriteHeader(http.StatusInternalServerError)
			return renderTeamTokens(c, d, team, "Failed to create token. Please try again.", "error", "")
		}

		// Render directly (no redirect) so we can show the plaintext once.
		return renderTeamTokens(c, d, team, "", "", plaintext)
	}
}

// TeamTokensRevoke handles POST /t/:team_name/tokens/:prefix/revoke.
func TeamTokensRevoke(d *TeamTokensDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := auth.ScopeFromEcho(c)
		if scope.User == nil {
			return c.Redirect(http.StatusSeeOther, "/users/log-in")
		}
		team, err := loadTeamAsMember(c, d.Teams, d.Logger)
		if err != nil {
			return err
		}

		prefix := c.Param("prefix")
		if err := d.Teams.RevokeAccessTokenByPrefix(c.Request().Context(), prefix); err != nil {
			d.Logger.Error("team tokens: RevokeAccessTokenByPrefix", "error", err, "prefix", prefix)
			// Still redirect — idempotent for already-revoked.
			return c.Redirect(http.StatusSeeOther, "/t/"+team.Name+"/tokens")
		}

		return c.Redirect(http.StatusSeeOther, "/t/"+team.Name+"/tokens")
	}
}
