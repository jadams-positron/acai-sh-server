package handlers

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/services"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// FeatureShowDeps groups dependencies for the feature detail page.
type FeatureShowDeps struct {
	Logger      *slog.Logger
	Teams       *teams.Repository
	FeatureView *services.FeatureViewService
}

// FeatureShow renders GET /t/:team_name/f/:feature_name.
func FeatureShow(d *FeatureShowDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := auth.ScopeFromEcho(c)
		if scope.User == nil {
			return c.Redirect(http.StatusSeeOther, "/users/log-in")
		}
		teamName := c.Param("team_name")
		featureName := c.Param("feature_name")

		team, err := d.Teams.GetByName(c.Request().Context(), teamName)
		if err != nil {
			if teams.IsNotFound(err) {
				return echo.NewHTTPError(http.StatusNotFound, "not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load team")
		}
		member, err := d.Teams.IsMember(c.Request().Context(), team.ID, scope.User.ID)
		if err != nil || !member {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}

		view, err := d.FeatureView.Resolve(c.Request().Context(), services.FeatureViewRequest{
			Team:        team,
			FeatureName: featureName,
		})
		if err != nil {
			d.Logger.Error("feature show: Resolve", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load feature")
		}

		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return views.FeatureShow(views.FeatureShowProps{
			Team:               team,
			FeatureName:        featureName,
			FeatureDescription: view.FeatureDescription,
			Cards:              view.Cards,
		}).Render(c.Request().Context(), c.Response())
	}
}
