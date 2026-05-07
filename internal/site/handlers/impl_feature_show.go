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
	Logger          *slog.Logger
	Teams           *teams.Repository
	FeatureView     *services.FeatureViewService
	FeatureStates   *services.FeatureStatesService
	Implementations *implementations.Repository
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
			CSRFToken:      csrfTokenFromEcho(c),
		}).Render(c.Request().Context(), c.Response())
	}
}

// ImplFeatureSetStatus handles POST
// /t/:team_name/i/:impl_slug/f/:feature_name/acid/:acid/status. Form params:
// status (one of the valid status values, or empty to clear). On success
// redirects 303 back to the drilldown page; on failure renders an HTTP
// error rather than re-rendering the page (the page is read-only on
// reload — the user resubmits via the select).
//
// Authorization: any team member can edit. Per-impl ownership is a
// future enhancement.
func ImplFeatureSetStatus(d *ImplFeatureShowDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := auth.ScopeFromEcho(c)
		if scope.User == nil {
			return c.Redirect(http.StatusSeeOther, "/users/log-in")
		}
		teamName := c.Param("team_name")
		implSlugParam := c.Param("impl_slug")
		featureName := c.Param("feature_name")
		acid := c.Param("acid")
		status := c.FormValue("status")

		team, err := d.Teams.GetByName(c.Request().Context(), teamName)
		if err != nil {
			if teams.IsNotFound(err) {
				return echo.NewHTTPError(http.StatusNotFound, "not found")
			}
			d.Logger.Error("impl feature set status: GetByName", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed")
		}
		member, err := d.Teams.IsMember(c.Request().Context(), team.ID, scope.User.ID)
		if err != nil || !member {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}

		implID := services.ParseImplSlug(implSlugParam)
		if implID == "" {
			return echo.NewHTTPError(http.StatusNotFound, "invalid implementation slug")
		}
		impl, err := d.Implementations.GetByID(c.Request().Context(), implID, team.ID)
		if err != nil {
			if errors.Is(err, implementations.ErrNotFound) {
				return echo.NewHTTPError(http.StatusNotFound, "not found")
			}
			d.Logger.Error("impl feature set status: GetByID", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed")
		}

		// Build a single-state update. Empty status clears the entry; any
		// other value is enum-validated by the service.
		stateInput := services.StateInput{}
		if status != "" {
			s := status
			stateInput.Status = &s
		}
		if _, err := d.FeatureStates.Update(c.Request().Context(), services.FeatureStatesUpdate{
			Team:               team,
			ProductName:        impl.ProductName,
			ImplementationName: impl.Name,
			FeatureName:        featureName,
			States:             map[string]services.StateInput{acid: stateInput},
			MaxStates:          1,
			MaxCommentLength:   0,
		}); err != nil {
			if errors.Is(err, services.ErrInvalidStatus) {
				return echo.NewHTTPError(http.StatusUnprocessableEntity, "invalid status")
			}
			d.Logger.Error("impl feature set status: FeatureStates.Update", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to update status")
		}

		return c.Redirect(http.StatusSeeOther,
			"/t/"+team.Name+"/i/"+implSlugParam+"/f/"+featureName)
	}
}
