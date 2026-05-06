package handlers

import (
	"log/slog"
	"net/http"
	"sort"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	"github.com/jadams-positron/acai-sh-server/internal/domain/specs"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// FeaturesIndexDeps groups dependencies for the cross-product features index.
type FeaturesIndexDeps struct {
	Logger   *slog.Logger
	Teams    *teams.Repository
	Products *products.Repository
	Specs    *specs.Repository
}

// FeaturesIndex renders GET /t/:team_name/features.
//
// Composes the page's data in Go: list the team's products, then for each
// product call ListDistinctFeatureNamesForProduct. Counts of products per
// feature are tracked alongside so we can surface "feature X lives in
// products A, B" without an extra schema/sqlc round trip. For typical team
// sizes (few products) this stays well under any per-request budget.
func FeaturesIndex(d *FeaturesIndexDeps) echo.HandlerFunc {
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
			d.Logger.Error("features index: GetByName", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load team")
		}
		member, err := d.Teams.IsMember(c.Request().Context(), team.ID, scope.User.ID)
		if err != nil || !member {
			return echo.NewHTTPError(http.StatusNotFound, "team not found")
		}

		prods, err := d.Products.ListForTeam(c.Request().Context(), team.ID)
		if err != nil {
			d.Logger.Error("features index: ListForTeam", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load products")
		}

		// featureName → set of products carrying it.
		productsPerFeature := map[string]map[string]struct{}{}
		for _, prod := range prods {
			names, err := d.Specs.ListDistinctFeatureNamesForProduct(c.Request().Context(), prod.ID)
			if err != nil {
				d.Logger.Error("features index: ListDistinctFeatureNamesForProduct", "error", err, "product", prod.Name)
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to load features")
			}
			for _, name := range names {
				if _, ok := productsPerFeature[name]; !ok {
					productsPerFeature[name] = map[string]struct{}{}
				}
				productsPerFeature[name][prod.Name] = struct{}{}
			}
		}

		// Flatten into a sorted slice for stable rendering.
		entries := make([]views.FeaturesIndexEntry, 0, len(productsPerFeature))
		for name, prodSet := range productsPerFeature {
			productNames := make([]string, 0, len(prodSet))
			for p := range prodSet {
				productNames = append(productNames, p)
			}
			sort.Strings(productNames)
			entries = append(entries, views.FeaturesIndexEntry{
				FeatureName:  name,
				ProductNames: productNames,
			})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].FeatureName < entries[j].FeatureName })

		shell, err := buildShellChrome(c, d.Teams, "Features · "+team.Name, team, "features", []views.Crumb{
			{Label: "Teams", HRef: "/teams"},
			{Label: team.Name, HRef: "/t/" + team.Name},
			{Label: "Features"},
		})
		if err != nil {
			d.Logger.Error("features index: shell chrome", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to load features")
		}

		c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
		return views.FeaturesIndex(views.FeaturesIndexProps{
			Shell:    shell,
			Team:     team,
			Features: entries,
		}).Render(c.Request().Context(), c.Response())
	}
}
