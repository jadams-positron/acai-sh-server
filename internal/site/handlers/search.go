package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/implementations"
	"github.com/jadams-positron/acai-sh-server/internal/domain/products"
	"github.com/jadams-positron/acai-sh-server/internal/domain/specs"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
)

// SearchDeps groups dependencies for the team search endpoint.
type SearchDeps struct {
	Logger          *slog.Logger
	Teams           *teams.Repository
	Products        *products.Repository
	Implementations *implementations.Repository
	Specs           *specs.Repository
}

// SearchResult is one row in any group.
type SearchResult struct {
	Label string `json:"label"`
	Href  string `json:"href"`
	Hint  string `json:"hint,omitempty"` // small secondary text (e.g. product name for impls)
}

// SearchResponse is the JSON shape consumed by the Cmd+K palette.
type SearchResponse struct {
	Products []SearchResult `json:"products"`
	Impls    []SearchResult `json:"impls"`
	Features []SearchResult `json:"features"`
	Branches []SearchResult `json:"branches"`
}

// Search renders GET /t/:team_name/search?q=<query>. Returns up to 5
// matches per group, scoped to the current team. Case-insensitive
// substring match on names — no FTS in v1; ACID-text search is out of
// scope for this iteration.
func Search(d *SearchDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := auth.ScopeFromEcho(c)
		if scope.User == nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}
		teamName := c.Param("team_name")
		team, err := d.Teams.GetByName(c.Request().Context(), teamName)
		if err != nil {
			if teams.IsNotFound(err) {
				return c.JSON(http.StatusNotFound, map[string]string{"error": "team not found"})
			}
			d.Logger.Error("search: GetByName", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed"})
		}
		member, err := d.Teams.IsMember(c.Request().Context(), team.ID, scope.User.ID)
		if err != nil || !member {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "team not found"})
		}

		q := strings.TrimSpace(c.QueryParam("q"))
		// Empty query: respond with empty groups so the palette can render
		// "type to search" without a round trip.
		if q == "" {
			return c.JSON(http.StatusOK, SearchResponse{})
		}
		needle := strings.ToLower(q)
		const maxPerGroup = 5

		out := SearchResponse{}

		// Products.
		prods, err := d.Products.ListForTeam(c.Request().Context(), team.ID)
		if err == nil {
			for _, p := range prods {
				if !strings.Contains(strings.ToLower(p.Name), needle) {
					continue
				}
				out.Products = append(out.Products, SearchResult{
					Label: p.Name,
					Href:  "/t/" + team.Name + "/p/" + p.Name,
				})
				if len(out.Products) >= maxPerGroup {
					break
				}
			}
		}

		// Implementations.
		impls, err := d.Implementations.List(c.Request().Context(), implementations.ListByTeamParams{TeamID: team.ID})
		if err == nil {
			for _, i := range impls {
				if !strings.Contains(strings.ToLower(i.Name), needle) {
					continue
				}
				out.Impls = append(out.Impls, SearchResult{
					Label: i.Name,
					Href:  "/t/" + team.Name + "/i/" + i.Name + "-" + strings.ReplaceAll(i.ID, "-", ""),
					Hint:  i.ProductName,
				})
				if len(out.Impls) >= maxPerGroup {
					break
				}
			}
		}

		// Features (cross-product).
		featureSet := map[string]string{} // featureName → first matching product
		for _, p := range prods {
			names, err := d.Specs.ListDistinctFeatureNamesForProduct(c.Request().Context(), p.ID)
			if err != nil {
				continue
			}
			for _, name := range names {
				if _, dup := featureSet[name]; dup {
					continue
				}
				if !strings.Contains(strings.ToLower(name), needle) {
					continue
				}
				featureSet[name] = p.Name
			}
		}
		for name, productName := range featureSet {
			out.Features = append(out.Features, SearchResult{
				Label: name,
				Href:  "/t/" + team.Name + "/f/" + name,
				Hint:  productName,
			})
			if len(out.Features) >= maxPerGroup {
				break
			}
		}

		// Branches.
		branches, err := d.Specs.ListBranchesForTeam(c.Request().Context(), team.ID)
		if err == nil {
			for _, b := range branches {
				if !strings.Contains(strings.ToLower(b.BranchName), needle) {
					continue
				}
				out.Branches = append(out.Branches, SearchResult{
					Label: b.BranchName,
					Href:  "/t/" + team.Name + "/branches",
					Hint:  b.RepoURI,
				})
				if len(out.Branches) >= maxPerGroup {
					break
				}
			}
		}

		c.Response().Header().Set("Cache-Control", "no-store")
		return json.NewEncoder(c.Response().Writer).Encode(out)
	}
}
