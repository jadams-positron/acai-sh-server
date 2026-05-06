package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// TeamsDeps groups the dependencies for team handlers.
type TeamsDeps struct {
	Logger *slog.Logger
	Teams  *teams.Repository
}

// TeamsIndex renders GET /teams: list of teams the user belongs to.
func TeamsIndex(d *TeamsDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := auth.ScopeFromEcho(c)
		if scope.User == nil {
			return c.Redirect(http.StatusSeeOther, "/users/log-in")
		}
		list, err := d.Teams.ListForUser(c.Request().Context(), scope.User.ID)
		if err != nil {
			d.Logger.Error("teams: ListForUser", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load teams")
		}
		shell, err := buildShellChrome(c, d.Teams, "Teams", nil, "", []views.Crumb{
			{Label: "Your teams"},
		})
		if err != nil {
			d.Logger.Error("teams: shell chrome", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load teams")
		}
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return views.TeamsIndex(views.TeamsIndexProps{
			Shell:     shell,
			Teams:     list,
			CSRFToken: csrfTokenFromEcho(c),
		}).Render(c.Request().Context(), c.Response())
	}
}

// TeamsCreate handles POST /teams.
func TeamsCreate(d *TeamsDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := auth.ScopeFromEcho(c)
		if scope.User == nil {
			return c.Redirect(http.StatusSeeOther, "/users/log-in")
		}
		name := c.FormValue("name")
		team, err := d.Teams.CreateTeamWithOwner(c.Request().Context(), scope.User.ID, name)
		if err != nil {
			// Re-render the page with an error banner.
			list, lerr := d.Teams.ListForUser(c.Request().Context(), scope.User.ID)
			if lerr != nil {
				d.Logger.Error("teams: re-list after create-error", "error", lerr)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to load teams")
			}
			flash := "Failed to create team."
			switch {
			case teams.IsInvalidTeamName(err):
				flash = "Team name must be alphanumeric (with hyphens/underscores), 1-64 chars."
			case errors.Is(err, teams.ErrDuplicateName):
				flash = "A team with that name already exists. Pick another."
			default:
				d.Logger.Error("teams: create", "error", err)
			}
			shell, sherr := buildShellChrome(c, d.Teams, "Teams", nil, "", []views.Crumb{
				{Label: "Your teams"},
			})
			if sherr != nil {
				d.Logger.Error("teams: shell chrome (after create-error)", "error", sherr)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to load teams")
			}
			c.Response().WriteHeader(http.StatusUnprocessableEntity)
			c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
			return views.TeamsIndex(views.TeamsIndexProps{
				Shell:         shell,
				Teams:         list,
				CSRFToken:     csrfTokenFromEcho(c),
				Flash:         flash,
				PrefilledName: name,
			}).Render(c.Request().Context(), c.Response())
		}
		return c.Redirect(http.StatusSeeOther, "/t/"+team.Name)
	}
}
