package handlers

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// ImplsIndexDeps groups dependencies for the implementations index page.
type ImplsIndexDeps struct {
	Logger          *slog.Logger
	Teams           *teams.Repository
	Implementations *implementations.Repository
}

// ImplsIndex renders GET /t/:team_name/implementations.
func ImplsIndex(d *ImplsIndexDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := auth.ScopeFromEcho(c)
		if scope.User == nil {
			return c.Redirect(http.StatusSeeOther, "/users/log-in")
		}
		teamName := c.Param("team_name")
		team, err := d.Teams.GetByName(c.Request().Context(), teamName)
		if err != nil {
			if teams.IsNotFound(err) {
				return echo.NewHTTPError(http.StatusNotFound, "team not found")
			}
			d.Logger.Error("impls index: GetByName", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load team")
		}
		member, err := d.Teams.IsMember(c.Request().Context(), team.ID, scope.User.ID)
		if err != nil || !member {
			return echo.NewHTTPError(http.StatusNotFound, "team not found")
		}

		impls, err := d.Implementations.List(c.Request().Context(), implementations.ListByTeamParams{
			TeamID: team.ID,
		})
		if err != nil {
			d.Logger.Error("impls index: List", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load implementations")
		}

		shell, err := buildShellChrome(c, d.Teams, "Implementations · "+team.Name, team, "implementations", []views.Crumb{
			{Label: "Teams", HRef: "/teams"},
			{Label: team.Name, HRef: "/t/" + team.Name},
			{Label: "Implementations"},
		})
		if err != nil {
			d.Logger.Error("impls index: shell chrome", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load implementations")
		}

		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return views.ImplsIndex(views.ImplsIndexProps{
			Shell:           shell,
			Team:            team,
			Implementations: impls,
		}).Render(c.Request().Context(), c.Response())
	}
}
