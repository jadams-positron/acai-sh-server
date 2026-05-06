package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/services"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// ImplFeatureShowDeps groups dependencies for the impl×feature drill-down page.
type ImplFeatureShowDeps struct {
	Logger      *slog.Logger
	Teams       *teams.Repository
	FeatureView *services.FeatureViewService
}

// ImplFeatureShow renders GET /t/:team_name/i/:impl_slug/f/:feature_name.
func ImplFeatureShow(d *ImplFeatureShowDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := auth.ScopeFromEcho(c)
		if scope.User == nil {
			return c.Redirect(http.StatusSeeOther, "/users/log-in")
		}
		teamName := c.Param("team_name")
		implSlug := c.Param("impl_slug")
		featureName := c.Param("feature_name")

		team, err := d.Teams.GetByName(c.Request().Context(), teamName)
		if err != nil {
			if teams.IsNotFound(err) {
				return echo.NewHTTPError(http.StatusNotFound, "not found")
			}
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

		view, err := d.FeatureView.ResolveImplFeatureView(c.Request().Context(), services.ImplFeatureViewRequest{
			Team:             team,
			ImplementationID: implID,
			FeatureName:      featureName,
		})
		if err != nil {
			if errors.Is(err, implementations.ErrNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "not found")
			}
			d.Logger.Error("impl feature show: ResolveImplFeatureView", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed")
		}

		shell, err := buildShellChrome(c, d.Teams, featureName+" · "+view.Implementation.Name, team, "overview", []views.Crumb{
			{Label: "Teams", HRef: "/teams"},
			{Label: team.Name, HRef: "/t/" + team.Name},
			{Label: view.Implementation.ProductName, HRef: "/t/" + team.Name + "/p/" + view.Implementation.ProductName},
			{Label: featureName, HRef: "/t/" + team.Name + "/f/" + featureName},
			{Label: view.Implementation.Name},
		})
		if err != nil {
			d.Logger.Error("impl feature show: shell chrome", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed")
		}
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return views.ImplFeature(views.ImplFeatureProps{
			Shell:          shell,
			Team:           team,
			Implementation: view.Implementation,
			ImplSlug:       implSlug,
			FeatureName:    featureName,
			Acids:          view.AcidEntries,
			HasSpec:        view.Spec != nil,
		}).Render(c.Request().Context(), c.Response())
	}
}
