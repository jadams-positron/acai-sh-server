package handlers

import (
	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// buildShellChrome assembles the cross-cutting shell props (user email,
// team list for the switcher, logout CSRF) from the request context. It
// calls Teams.ListForUser and is intended to be invoked once per page
// render. Returns an error only on DB failure; pages should bubble it up
// as a 500.
//
// activeTeam, activeSection, title, and crumbs are page-specific and are
// passed in by the caller.
func buildShellChrome(
	c echo.Context,
	teamsRepo *teams.Repository,
	title string,
	activeTeam *teams.Team,
	activeSection string,
	crumbs []views.Crumb,
) (views.ShellProps, error) {
	scope := auth.ScopeFromEcho(c)
	var email string
	var userID string
	if scope.User != nil {
		email = scope.User.Email
		userID = scope.User.ID
	}

	var userTeams []*teams.Team
	if userID != "" {
		ts, err := teamsRepo.ListForUser(c.Request().Context(), userID)
		if err != nil {
			return views.ShellProps{}, err
		}
		userTeams = ts
	}

	return views.ShellProps{
		Title:           title,
		UserEmail:       email,
		Teams:           userTeams,
		ActiveTeam:      activeTeam,
		ActiveSection:   activeSection,
		LogoutCSRFToken: csrfTokenFromEcho(c),
		Crumbs:          crumbs,
	}, nil
}
