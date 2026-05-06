package handlers

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	"github.com/jadams-positron/acai-sh-server/internal/domain/specs"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/services"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// ProductShowDeps groups dependencies for the product detail page.
type ProductShowDeps struct {
	Logger          *slog.Logger
	Teams           *teams.Repository
	Products        *products.Repository
	Implementations *implementations.Repository
	Specs           *specs.Repository
	FeatureView     *services.FeatureViewService
}

// ProductShow renders GET /t/:team_name/p/:product_name.
func ProductShow(d *ProductShowDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := auth.ScopeFromEcho(c)
		if scope.User == nil {
			return c.Redirect(http.StatusSeeOther, "/users/log-in")
		}
		teamName := c.Param("team_name")
		productName := c.Param("product_name")

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

		prod, err := d.Products.GetByTeamAndName(c.Request().Context(), team.ID, productName)
		if err != nil {
			if products.IsNotFound(err) {
				return echo.NewHTTPError(http.StatusNotFound, "not found")
			}
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load product")
		}

		// One service call composes both the per-impl summaries and the
		// per-feature roll-ups by reusing ResolveImplOverview internally.
		overview, err := d.FeatureView.ResolveProductOverview(c.Request().Context(), services.ProductOverviewRequest{
			TeamID:    team.ID,
			ProductID: prod.ID,
		})
		if err != nil {
			d.Logger.Error("product show: ResolveProductOverview", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load product overview")
		}

		shell, err := buildShellChrome(c, d.Teams, prod.Name+" · "+team.Name, team, "overview", []views.Crumb{
			{Label: "Teams", HRef: "/teams"},
			{Label: team.Name, HRef: "/t/" + team.Name},
			{Label: prod.Name},
		})
		if err != nil {
			d.Logger.Error("product show: shell chrome", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load product")
		}
		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return views.ProductShow(views.ProductShowProps{
			Shell:    shell,
			Team:     team,
			Product:  prod,
			Overview: overview,
		}).Render(c.Request().Context(), c.Response())
	}
}
