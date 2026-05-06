package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/specs"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/services"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// ImplShowDeps groups dependencies for the implementation detail page.
type ImplShowDeps struct {
	Logger          *slog.Logger
	Teams           *teams.Repository
	Implementations *implementations.Repository
	Specs           *specs.Repository
	FeatureView     *services.FeatureViewService
}

// ImplShow renders GET /t/:team_name/i/:impl_slug.
func ImplShow(d *ImplShowDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := auth.ScopeFromEcho(c)
		if scope.User == nil {
			return c.Redirect(http.StatusSeeOther, "/users/log-in")
		}
		teamName := c.Param("team_name")
		implSlug := c.Param("impl_slug")

		team, err := d.Teams.GetByName(c.Request().Context(), teamName)
		if err != nil {
			if teams.IsNotFound(err) {
				return echo.NewHTTPError(http.StatusNotFound, "not found")
			}
			d.Logger.Error("impl show: GetByName", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed")
		}
		member, err := d.Teams.IsMember(c.Request().Context(), team.ID, scope.User.ID)
		if err != nil || !member {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}

		implID := services.ParseImplSlug(implSlug)
		if implID == "" {
			return echo.NewHTTPError(http.StatusNotFound, "invalid implementation slug")
		}
		impl, err := d.Implementations.GetByID(c.Request().Context(), implID, team.ID)
		if err != nil {
			if errors.Is(err, implementations.ErrNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "not found")
			}
			d.Logger.Error("impl show: GetByID", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed")
		}

		// Parent impl (optional).
		var parent *implementations.Implementation
		if impl.ParentImplementationID != nil {
			pp, perr := d.Implementations.GetByID(c.Request().Context(), *impl.ParentImplementationID, team.ID)
			if perr != nil && !errors.Is(perr, implementations.ErrNotFound) {
				d.Logger.Error("impl show: parent GetByID", "error", perr)
				// Soft fail — render without parent rather than 500.
			} else if perr == nil {
				parent = pp
			}
		}

		// Tracked branches — surfaced as a small section above the feature list
		// so users see which branches feed this impl's data.
		trackedBranches, err := d.Specs.ListTrackedBranchesForImpl(c.Request().Context(), impl.ID)
		if err != nil {
			d.Logger.Error("impl show: ListTrackedBranchesForImpl", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load tracked branches")
		}

		// Per-feature progress overview (counts by status), plus the aggregate
		// roll-up shown in the page header banner.
		overview, err := d.FeatureView.ResolveImplOverview(c.Request().Context(), services.ImplOverviewRequest{
			Implementation: impl,
		})
		if err != nil {
			d.Logger.Error("impl show: ResolveImplOverview", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load impl overview")
		}

		shell, err := buildShellChrome(c, d.Teams, impl.Name+" · "+team.Name, team, "implementations", []views.Crumb{
			{Label: "Teams", HRef: "/teams"},
			{Label: team.Name, HRef: "/t/" + team.Name},
			{Label: "Implementations", HRef: "/t/" + team.Name + "/implementations"},
			{Label: impl.ProductName, HRef: "/t/" + team.Name + "/p/" + impl.ProductName},
			{Label: impl.Name},
		})
		if err != nil {
			d.Logger.Error("impl show: shell chrome", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed")
		}

		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return views.ImplShow(views.ImplShowProps{
			Shell:           shell,
			Team:            team,
			Implementation:  impl,
			ImplSlug:        implSlug,
			Parent:          parent,
			Overview:        overview,
			TrackedBranches: trackedBranches,
		}).Render(c.Request().Context(), c.Response())
	}
}
