defmodule AcaiWeb.RouterTest do
  use ExUnit.Case, async: true

  defp dashboard_session_name(route) do
    {_view, _action, _opts, live_session} = route.metadata[:phoenix_live_view]
    live_session.name
  end

  describe "dashboard routes" do
    # dashboard.ROUTING.1
    # dashboard.ROUTING.1-note
    test "dashboard.ROUTING.1-note keeps the dev dashboard default session while admin uses a distinct one" do
      routes = AcaiWeb.Router.__routes__()

      dev_routes =
        Enum.filter(routes, fn route ->
          String.starts_with?(route.path, "/dev/dashboard") and
            route.plug == Phoenix.LiveView.Plug
        end)

      admin_routes =
        Enum.filter(routes, fn route ->
          String.starts_with?(route.path, "/admin/dashboard") and
            route.plug == Phoenix.LiveView.Plug
        end)

      assert dev_routes != []
      assert admin_routes != []

      dev_session_names = MapSet.new(dev_routes, &dashboard_session_name/1)
      admin_session_names = MapSet.new(admin_routes, &dashboard_session_name/1)

      assert dev_session_names == MapSet.new([:live_dashboard])
      assert admin_session_names == MapSet.new([:admin_live_dashboard])
      assert MapSet.disjoint?(dev_session_names, admin_session_names)
    end
  end
end
