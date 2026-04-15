defmodule AcaiWeb.Live.Components.FeatureStatusDropdown do
  @moduledoc false

  use AcaiWeb, :html

  alias Phoenix.LiveView.JS

  @status_metadata %{
    nil => %{label: "No status", badge_class: "badge-ghost"},
    "assigned" => %{label: "assigned", badge_class: "badge-warning"},
    "blocked" => %{label: "blocked", badge_class: "badge-error"},
    "incomplete" => %{label: "incomplete", badge_class: "badge-neutral"},
    "completed" => %{label: "completed", badge_class: "badge-info"},
    "rejected" => %{label: "rejected", badge_class: "badge-error"},
    "accepted" => %{label: "accepted", badge_class: "badge-success"}
  }

  @valid_statuses ["assigned", "blocked", "incomplete", "completed", "rejected", "accepted"]

  # feature-impl-view.LIST.3-1: Canonical badge for status trigger and list options
  def feature_status_badge(assigns) do
    metadata = Map.get(@status_metadata, assigns.status, @status_metadata[nil])

    assigns =
      assigns
      |> assign(:badge_class, metadata.badge_class)
      |> assign(:label, metadata.label)

    ~H"""
    <span class={[
      "badge badge-sm",
      @inherited && "badge-soft",
      @badge_class
    ]}>
      {@label}
    </span>
    """
  end

  # feature-impl-view.LIST.3-1: Shared dropdown renderer for table and drawer
  def feature_status_dropdown(assigns) do
    assigns =
      assigns
      |> assign_new(:id_prefix, fn -> "status" end)
      |> assign_new(:open_upward, fn -> false end)
      |> assign_new(:inherited, fn -> false end)
      |> assign_new(:valid_statuses, fn -> @valid_statuses end)

    ~H"""
    <div class="relative inline-block">
      <% acid_dom = acid_dom_id(@acid) %>
      <% dropdown_id = "#{@id_prefix}-dropdown-#{acid_dom}" %>
      <% trigger_id = "#{@id_prefix}-trigger-#{acid_dom}" %>
      <% toggle_dropdown_js = JS.toggle(to: "##{dropdown_id}") %>
      <% close_dropdown_js = JS.hide(to: "##{dropdown_id}") %>

      <button
        type="button"
        id={trigger_id}
        class="cursor-pointer transition-opacity hover:opacity-80 phx-click-loading:opacity-60"
        phx-click={toggle_dropdown_js}
        phx-stop-propagation
      >
        <.feature_status_badge status={@current_status} inherited={@inherited} />
      </button>

      <div
        id={dropdown_id}
        class={[
          "absolute z-50 hidden w-40 rounded-lg border border-base-300 bg-base-100 py-1 shadow-lg",
          if(@open_upward, do: "bottom-full mb-1", else: "mt-1")
        ]}
        phx-click-away={close_dropdown_js}
        phx-stop-propagation
      >
        <button
          type="button"
          data-status="none"
          class={[
            "flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition-colors hover:bg-base-200 phx-click-loading:opacity-60",
            is_nil(@current_status) && "bg-base-200"
          ]}
          phx-click={select_status_js(close_dropdown_js, trigger_id, @acid, nil)}
          phx-stop-propagation
        >
          <span class="badge badge-sm badge-ghost text-base-content/50">No status</span>
        </button>

        <%= for status <- @valid_statuses do %>
          <button
            type="button"
            data-status={status}
            class={[
              "flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition-colors hover:bg-base-200 phx-click-loading:opacity-60",
              @current_status == status && "bg-base-200"
            ]}
            phx-click={select_status_js(close_dropdown_js, trigger_id, @acid, status)}
            phx-stop-propagation
          >
            <.feature_status_badge status={status} inherited={false} />
            <%= if @current_status == status do %>
              <.icon name="hero-check" class="ml-auto size-4 text-success" />
            <% end %>
          </button>
        <% end %>
      </div>
    </div>
    """
  end

  def valid_statuses, do: @valid_statuses

  defp acid_dom_id(acid), do: String.replace(acid, ".", "-")

  defp select_status_js(close_dropdown_js, trigger_id, acid, status) do
    close_dropdown_js
    |> JS.push("select_status",
      value: %{acid: acid, status: status || ""},
      loading: "##{trigger_id}"
    )
  end
end
