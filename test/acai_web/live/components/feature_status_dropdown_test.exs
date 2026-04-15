defmodule AcaiWeb.Live.Components.FeatureStatusDropdownTest do
  use AcaiWeb.ConnCase, async: true

  import Phoenix.LiveViewTest

  alias AcaiWeb.Live.Components.FeatureStatusDropdown

  defp render_dropdown(assigns) do
    render_component(&FeatureStatusDropdown.feature_status_dropdown/1, assigns)
  end

  describe "feature-impl-view.DRAWER.3-1, feature-impl-view.DRAWER.3-2, and feature-impl-view.DRAWER.3-3" do
    test "renders the shared trigger, always-present menu, and downward drawer placement by default" do
      html =
        render_dropdown(%{
          acid: "my-feature.COMP.1",
          current_status: "completed",
          inherited: false,
          id_prefix: "drawer-status"
        })

      assert html =~ ~s(id="drawer-status-trigger-my-feature-COMP-1")
      assert html =~ ~s(id="drawer-status-dropdown-my-feature-COMP-1")
      assert html =~ ~s(phx-click-away=)
      assert html =~ ~s(phx-click-loading:opacity-60)
      refute html =~ "open_status_dropdown"
      refute html =~ "close_status_dropdown"
      assert html =~ "No status"
      assert html =~ "hero-check"
      assert html =~ "mt-1"
      assert html =~ "hidden"
      refute html =~ "bottom-full"
    end

    test "marks the No status option when the current status is nil" do
      html =
        render_dropdown(%{
          acid: "my-feature.COMP.1",
          current_status: nil,
          inherited: false,
          id_prefix: "drawer-status"
        })

      assert html =~ "No status"
      assert html =~ "bg-base-200"
      refute html =~ "hero-check"
    end
  end

  describe "feature-impl-view.LIST.3-1" do
    test "renders upward placement for table rows near the bottom" do
      html =
        render_dropdown(%{
          acid: "my-feature.COMP.1",
          current_status: "blocked",
          inherited: true,
          open_upward: true,
          id_prefix: "status"
        })

      assert html =~ ~s(id="status-dropdown-my-feature-COMP-1")
      assert html =~ "bottom-full"
      refute html =~ "mt-1"
    end
  end
end
