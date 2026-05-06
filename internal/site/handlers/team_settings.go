package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jadams-positron/acai-sh-server/internal/auth"
	"github.com/jadams-positron/acai-sh-server/internal/domain/accounts"
	"github.com/jadams-positron/acai-sh-server/internal/domain/teams"
	"github.com/jadams-positron/acai-sh-server/internal/site/views"
)

// TeamSettingsDeps groups dependencies for the team settings page.
type TeamSettingsDeps struct {
	Logger   *slog.Logger
	Teams    *teams.Repository
	Accounts *accounts.Repository
}

// loadTeamAndCheckOwner is a helper that loads the team by name, verifies the
// current user is a member, and returns (team, role, error). On any hard
// failure it writes the appropriate HTTP error to c and returns a non-nil err.
func loadTeamAndCheckOwner(c echo.Context, d *TeamSettingsDeps) (*teams.Team, string, error) {
	scope := auth.ScopeFromEcho(c)
	if scope.User == nil {
		if err := c.Redirect(http.StatusSeeOther, "/users/log-in"); err != nil {
			return nil, "", err
		}
		return nil, "", echo.ErrUnauthorized
	}
	teamName := c.Param("team_name")
	team, err := d.Teams.GetByName(c.Request().Context(), teamName)
	if err != nil {
		if teams.IsNotFound(err) {
			return nil, "", echo.NewHTTPError(http.StatusNotFound, "team not found")
		}
		d.Logger.Error("team settings: GetByName", "error", err)
		return nil, "", echo.NewHTTPError(http.StatusInternalServerError, "failed to load team")
	}
	role, err := d.Teams.GetMemberRole(c.Request().Context(), team.ID, scope.User.ID)
	if err != nil {
		if teams.IsNotFound(err) {
			return nil, "", echo.NewHTTPError(http.StatusNotFound, "team not found")
		}
		d.Logger.Error("team settings: GetMemberRole", "error", err)
		return nil, "", echo.NewHTTPError(http.StatusInternalServerError, "failed to verify membership")
	}
	return team, role, nil
}

// renderTeamSettings is the shared render helper for the settings page.
func renderTeamSettings(c echo.Context, d *TeamSettingsDeps, team *teams.Team, role, flash, flashType string) error {
	scope := auth.ScopeFromEcho(c)
	members, err := d.Teams.ListMembers(c.Request().Context(), team.ID)
	if err != nil {
		d.Logger.Error("team settings: ListMembers", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load members")
	}
	currentUserID := ""
	if scope.User != nil {
		currentUserID = scope.User.ID
	}
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	return views.TeamSettings(views.TeamSettingsProps{
		Team:          team,
		Members:       members,
		CSRFToken:     csrfTokenFromEcho(c),
		Flash:         flash,
		FlashType:     flashType,
		CanAdmin:      role == "owner",
		CurrentUserID: currentUserID,
	}).Render(c.Request().Context(), c.Response())
}

// TeamSettings renders GET /t/:team_name/settings.
func TeamSettings(d *TeamSettingsDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := auth.ScopeFromEcho(c)
		if scope.User == nil {
			return c.Redirect(http.StatusSeeOther, "/users/log-in")
		}
		team, role, err := loadTeamAndCheckOwner(c, d)
		if err != nil {
			return err
		}
		return renderTeamSettings(c, d, team, role, "", "")
	}
}

// TeamSettingsAddMember handles POST /t/:team_name/settings/members.
func TeamSettingsAddMember(d *TeamSettingsDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := auth.ScopeFromEcho(c)
		if scope.User == nil {
			return c.Redirect(http.StatusSeeOther, "/users/log-in")
		}
		team, role, err := loadTeamAndCheckOwner(c, d)
		if err != nil {
			return err
		}
		if role != "owner" {
			return echo.NewHTTPError(http.StatusForbidden, "only owners can invite members")
		}

		email := c.FormValue("email")
		inviteRole := c.FormValue("role")

		// Look up existing user.
		user, err := d.Accounts.GetUserByEmail(c.Request().Context(), email)
		if err != nil {
			if accounts.IsNotFound(err) {
				c.Response().WriteHeader(http.StatusUnprocessableEntity)
				return renderTeamSettings(c, d, team, role,
					"No account with that email — the user needs to register first.", "error")
			}
			d.Logger.Error("team settings: GetUserByEmail", "error", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to look up user")
		}

		if err := d.Teams.AddMember(c.Request().Context(), team.ID, user.ID, inviteRole); err != nil {
			flash := "Failed to invite member."
			switch {
			case teams.IsAlreadyMember(err):
				flash = "That user is already a member of this team."
			case errors.Is(err, teams.ErrInvalidRole):
				flash = "Invalid role. Must be one of: owner, maintainer, developer, member."
			default:
				d.Logger.Error("team settings: AddMember", "error", err)
			}
			c.Response().WriteHeader(http.StatusUnprocessableEntity)
			return renderTeamSettings(c, d, team, role, flash, "error")
		}

		return c.Redirect(http.StatusSeeOther, "/t/"+team.Name+"/settings")
	}
}

// TeamSettingsRemoveMember handles POST /t/:team_name/settings/members/:user_id/remove.
func TeamSettingsRemoveMember(d *TeamSettingsDeps) echo.HandlerFunc {
	return func(c echo.Context) error {
		scope := auth.ScopeFromEcho(c)
		if scope.User == nil {
			return c.Redirect(http.StatusSeeOther, "/users/log-in")
		}
		team, role, err := loadTeamAndCheckOwner(c, d)
		if err != nil {
			return err
		}

		targetUserID := c.Param("user_id")

		// Authorization: only an owner OR the user themselves can remove.
		isSelf := scope.User.ID == targetUserID
		if role != "owner" && !isSelf {
			return echo.NewHTTPError(http.StatusForbidden, "only owners can remove members")
		}

		if err := d.Teams.RemoveMember(c.Request().Context(), team.ID, targetUserID); err != nil {
			flash := "Failed to remove member."
			if teams.IsLastOwner(err) {
				flash = "Cannot remove the last owner of a team."
			} else {
				d.Logger.Error("team settings: RemoveMember", "error", err)
			}
			c.Response().WriteHeader(http.StatusUnprocessableEntity)
			return renderTeamSettings(c, d, team, role, flash, "error")
		}

		return c.Redirect(http.StatusSeeOther, "/t/"+team.Name+"/settings")
	}
}
