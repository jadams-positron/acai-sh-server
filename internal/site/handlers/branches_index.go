package handlers

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/specs"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// BranchesIndexDeps groups dependencies for the team branches index.
type BranchesIndexDeps struct {
	Logger          *slog.Logger
	Teams           *teams.Repository
	Specs           *specs.Repository
	Implementations *implementations.Repository
}

// BranchesIndex renders GET /t/:team_name/branches.
//
// Lists all branches under the team alongside the implementations tracking
// each. Branch counts are bounded by team activity (typically tens), so the
// per-branch fan-out call to Implementations.List(by-branch) is acceptable
// without a tailored sqlc join.
func BranchesIndex(d *BranchesIndexDeps) echo.HandlerFunc {
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
			d.Logger.Error("branches index: GetByName", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load team")
		}
		member, err := d.Teams.IsMember(c.Request().Context(), team.ID, scope.User.ID)
		if err != nil || !member {
			return echo.NewHTTPError(http.StatusNotFound, "team not found")
		}

		branches, err := d.Specs.ListBranchesForTeam(c.Request().Context(), team.ID)
		if err != nil {
			d.Logger.Error("branches index: ListBranchesForTeam", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load branches")
		}

		entries := make([]views.BranchesIndexEntry, 0, len(branches))
		for _, b := range branches {
			impls, err := d.Implementations.List(c.Request().Context(), implementations.ListByTeamParams{
				TeamID:     team.ID,
				RepoURI:    &b.RepoURI,
				BranchName: &b.BranchName,
			})
			if err != nil {
				d.Logger.Error("branches index: List(by-branch)", "error", err, "branch", b.BranchName)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to load tracked impls")
			}
			entries = append(entries, views.BranchesIndexEntry{Branch: b, Implementations: impls})
		}

		shell, err := buildShellChrome(c, d.Teams, "Branches · "+team.Name, team, "branches", []views.Crumb{
			{Label: "Teams", HRef: "/teams"},
			{Label: team.Name, HRef: "/t/" + team.Name},
			{Label: "Branches"},
		})
		if err != nil {
			d.Logger.Error("branches index: shell chrome", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load branches")
		}

		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return views.BranchesIndex(views.BranchesIndexProps{
			Shell:    shell,
			Team:     team,
			Branches: entries,
		}).Render(c.Request().Context(), c.Response())
	}
}
