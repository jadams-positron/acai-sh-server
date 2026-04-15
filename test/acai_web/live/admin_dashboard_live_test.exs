defmodule AcaiWeb.AdminDashboardLiveTest do
  use AcaiWeb.ConnCase, async: true

  import Phoenix.LiveViewTest
  import Acai.AccountsFixtures
  import Acai.DataModelFixtures

  defp fresh_authenticated_at do
    DateTime.utc_now(:second) |> DateTime.add(-5, :minute)
  end

  describe "/admin/dashboard" do
    # dashboard.AUTH.3
    test "dashboard.AUTH.3 redirects unauthenticated users to log-in", %{conn: conn} do
      assert {:error, {:redirect, %{to: path}}} = live(conn, ~p"/admin/dashboard")
      assert path == ~p"/users/log-in"
    end

    # dashboard.AUTH.2
    # dashboard.AUTH.2-1
    test "dashboard.AUTH.2-1 redirects stale sessions to log-in", %{conn: conn} do
      user = user_fixture()
      team = team_fixture(%{global_admin: true})
      user_team_role_fixture(team, user, %{title: "owner"})

      stale_authenticated_at = DateTime.utc_now(:second) |> DateTime.add(-21, :minute)
      conn = log_in_user(conn, user, token_authenticated_at: stale_authenticated_at)

      assert {:error, {:redirect, %{to: path, flash: flash}}} =
               live(conn, ~p"/admin/dashboard/home")

      assert path == ~p"/users/log-in"
      assert flash["error"] == "You must re-authenticate to access this page."
    end

    # dashboard.AUTH.4
    test "dashboard.AUTH.4 redirects fresh non-whitelisted users to /teams", %{conn: conn} do
      user = user_fixture()
      team = team_fixture(%{global_admin: false})
      user_team_role_fixture(team, user, %{title: "owner"})

      conn = log_in_user(conn, user, token_authenticated_at: fresh_authenticated_at())

      assert {:error, {:redirect, %{to: path}}} = live(conn, ~p"/admin/dashboard/home")
      assert path == ~p"/teams"
    end

    # dashboard.AUTH.1-1
    # dashboard.ROUTING.1
    test "dashboard.AUTH.1-1 allows a readonly member of a global admin team to load the dashboard",
         %{conn: conn} do
      user = user_fixture()
      team = team_fixture(%{global_admin: true})
      user_team_role_fixture(team, user, %{title: "readonly"})

      conn = log_in_user(conn, user, token_authenticated_at: fresh_authenticated_at())

      {:ok, view, _html} = live(conn, ~p"/admin/dashboard/home")

      assert has_element?(view, "#menu")
      assert has_element?(view, "#menu-bar")
      assert has_element?(view, "#main")
    end
  end
end
