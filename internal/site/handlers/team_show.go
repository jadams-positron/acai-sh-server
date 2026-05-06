package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// TeamShowDeps groups dependencies for the team detail page.
type TeamShowDeps struct {
	Logger   *slog.Logger
	Teams    *teams.Repository
	Products *products.Repository
}

// TeamShow renders GET /t/:team_name.
func TeamShow(d *TeamShowDeps) echo.HandlerFunc {
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
			d.Logger.Error("team show: GetByName", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load team")
		}

		// Authorization: only members see the page (don't leak existence to non-members).
		member, err := d.Teams.IsMember(c.Request().Context(), team.ID, scope.User.ID)
		if err != nil {
			d.Logger.Error("team show: IsMember", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to verify membership")
		}
		if !member {
			return echo.NewHTTPError(http.StatusNotFound, "team not found")
		}

		prods, err := d.Products.ListForTeam(c.Request().Context(), team.ID)
		if err != nil {
			d.Logger.Error("team show: ListForTeam", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load products")
		}

		members, err := d.Teams.ListMembers(c.Request().Context(), team.ID)
		if err != nil {
			d.Logger.Error("team show: ListMembers", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load members")
		}

		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return views.TeamShow(views.TeamShowProps{
			Team:      team,
			Products:  prods,
			Members:   members,
			CSRFToken: csrfTokenFromEcho(c),
		}).Render(c.Request().Context(), c.Response())
	}
}

// TeamCreateProduct handles POST /t/:team_name/products.
func TeamCreateProduct(d *TeamShowDeps) echo.HandlerFunc {
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
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load team")
		}
		member, err := d.Teams.IsMember(c.Request().Context(), team.ID, scope.User.ID)
		if err != nil || !member {
			return echo.NewHTTPError(http.StatusNotFound, "team not found")
		}

		name := c.FormValue("name")
		prod, err := d.Products.Create(c.Request().Context(), team.ID, name)
		if err != nil {
			// Re-render the page with an error.
			prods, _ := d.Products.ListForTeam(c.Request().Context(), team.ID)
			members, _ := d.Teams.ListMembers(c.Request().Context(), team.ID)
			flash := "Failed to create product."
			switch {
			case products.IsInvalidProductName(err):
				flash = "Product name must be alphanumeric (with hyphens/underscores), 1-64 chars."
			case products.IsDuplicateName(err) || errors.Is(err, products.ErrDuplicateName):
				flash = "A product with that name already exists. Pick another."
			default:
				d.Logger.Error("team show: create product", "error", err)
			}
			c.Response().WriteHeader(http.StatusUnprocessableEntity)
			c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
			return views.TeamShow(views.TeamShowProps{
				Team:                 team,
				Products:             prods,
				Members:              members,
				CSRFToken:            csrfTokenFromEcho(c),
				Flash:                flash,
				PrefilledProductName: name,
			}).Render(c.Request().Context(), c.Response())
		}

		return c.Redirect(http.StatusSeeOther, "/t/"+team.Name+"/p/"+prod.Name)
	}
}
