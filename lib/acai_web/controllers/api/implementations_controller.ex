defmodule AcaiWeb.Api.ImplementationsController do
  @moduledoc """
  Read-only API controller for implementation discovery.
  """

  use AcaiWeb.Api.Controller

  alias Acai.Products
  alias Acai.Implementations
  alias AcaiWeb.Api.Schemas.ReadSchemas

  # implementations.ENDPOINT.1, implementations.ENDPOINT.2
  tags(["Actions"])
  security([%{"bearerAuth" => []}])

  operation(:index,
    summary: "List implementations",
    description:
      "Discover implementations either within one Product or across Products for an exact Repo + Branch.
      This is the orientation endpoint the CLI uses to resolve which implementation contexts track the current branch before any single-context detail reads.",
    parameters: [
      # implementations.REQUEST.1
      OpenApiSpex.Operation.parameter(:product_name, :query, :string, "Product name",
        required: false
      ),
      OpenApiSpex.Operation.parameter(:repo_uri, :query, :string, "Exact repository URI",
        required: false
      ),
      OpenApiSpex.Operation.parameter(:branch_name, :query, :string, "Exact branch name",
        required: false
      ),
      OpenApiSpex.Operation.parameter(
        :feature_name,
        :query,
        :string,
        "Filter to implementations that can resolve this feature",
        required: false
      )
    ],
    responses: [
      ok: {"Implementation list", "application/json", ReadSchemas.ImplementationsResponse},
      unauthorized: {"Unauthorized", "application/json", ReadSchemas.ErrorResponse},
      forbidden: {"Forbidden", "application/json", ReadSchemas.ErrorResponse},
      unprocessable_entity: {"Validation error", "application/json", ReadSchemas.ErrorResponse}
    ]
  )

  # implementations.RESPONSE.1, implementations.RESPONSE.8, implementations.RESPONSE.9, implementations.RESPONSE.10
  def index(conn, params) do
    token = conn.assigns.current_token
    team = conn.assigns.current_team
    request_params = merged_params(conn, params)

    with :ok <- ensure_scope(token, "impls:read"),
         {:ok, parsed} <- parse_params(request_params),
         :ok <- ensure_feature_scope(token, parsed.feature_name),
         {:ok, payload} <- build_payload(team, parsed) do
      render_data(conn, payload)
    else
      {:error, reason} -> {:error, reason}
    end
  end

  defp build_payload(team, %{
         product_name: nil,
         repo_uri: repo_uri,
         branch_name: branch_name,
         feature_name: feature_name
       }) do
    # implementations.REQUEST.1-1, implementations.FILTERS.1-1, implementations.FILTERS.2, implementations.FILTERS.5, implementations.FILTERS.7, implementations.RESPONSE.3, implementations.RESPONSE.4, implementations.RESPONSE.5, implementations.RESPONSE.6-1
    implementations =
      Implementations.list_api_implementations_by_branch(team, repo_uri, branch_name,
        feature_name: feature_name
      )

    {:ok,
     %{
       repo_uri: repo_uri,
       branch_name: branch_name,
       implementations: build_implementation_entries(implementations)
     }}
  end

  defp build_payload(team, %{
         product_name: product_name,
         repo_uri: repo_uri,
         branch_name: branch_name,
         feature_name: feature_name
       }) do
    # implementations.FILTERS.1, implementations.FILTERS.2, implementations.FILTERS.5, implementations.RESPONSE.2, implementations.RESPONSE.3, implementations.RESPONSE.4, implementations.RESPONSE.5
    branch_filter = if repo_uri && branch_name, do: {repo_uri, branch_name}, else: nil

    implementations =
      case Products.get_product_by_team_and_name(team, product_name) do
        {:ok, product} ->
          Implementations.list_api_implementations(team, product,
            branch_filter: branch_filter,
            feature_name: feature_name
          )

        {:error, :not_found} ->
          []
      end

    data =
      %{
        product_name: product_name,
        implementations: build_implementation_entries(implementations)
      }
      |> maybe_put_branch_filter(repo_uri, branch_name)

    {:ok, data}
  end

  defp maybe_put_branch_filter(data, nil, nil), do: data

  defp maybe_put_branch_filter(data, repo_uri, branch_name),
    do: Map.merge(data, %{repo_uri: repo_uri, branch_name: branch_name})

  defp merged_params(conn, params) do
    conn.query_params
    |> Map.merge(conn.body_params || %{})
    |> Map.merge(params || %{})
  end

  defp ensure_feature_scope(_token, nil), do: :ok

  defp ensure_feature_scope(token, _feature_name) when is_map(token) do
    # implementations.AUTH.3, implementations.ENDPOINT.2, implementations.FILTERS.6
    if Acai.Teams.token_has_scope?(token, "specs:read"),
      do: :ok,
      else: {:error, {:forbidden, "Token missing required scope: specs:read"}}
  end

  defp ensure_scope(token, scope) do
    if Acai.Teams.token_has_scope?(token, scope),
      do: :ok,
      else: {:error, {:forbidden, "Token missing required scope: #{scope}"}}
  end

  defp parse_params(params) do
    # implementations.REQUEST.1, implementations.REQUEST.1-1, implementations.REQUEST.2, implementations.REQUEST.3, implementations.REQUEST.4, implementations.FILTERS.3, implementations.FILTERS.4, implementations.VALIDATION.1, implementations.VALIDATION.1-1, implementations.VALIDATION.2, implementations.VALIDATION.3, implementations.RESPONSE.8
    with {:ok, product_name} <- optional_string(params, "product_name"),
         {:ok, repo_uri} <- optional_string(params, "repo_uri"),
         {:ok, branch_name} <- optional_string(params, "branch_name"),
         {:ok, feature_name} <- optional_string(params, "feature_name"),
         :ok <- validate_lookup_mode(product_name, repo_uri, branch_name) do
      {:ok,
       %{
         product_name: product_name,
         repo_uri: repo_uri,
         branch_name: branch_name,
         feature_name: feature_name
       }}
    end
  end

  defp validate_branch_pair(nil, nil), do: :ok

  defp validate_branch_pair(_repo_uri, nil),
    do: {:error, "branch_name is required when repo_uri is provided"}

  defp validate_branch_pair(nil, _branch_name),
    do: {:error, "repo_uri is required when branch_name is provided"}

  defp validate_branch_pair(_repo_uri, _branch_name), do: :ok

  defp validate_lookup_mode(product_name, repo_uri, branch_name) do
    with :ok <- validate_branch_pair(repo_uri, branch_name),
         :ok <- validate_product_optional_branch_requirement(product_name, repo_uri, branch_name) do
      :ok
    end
  end

  defp validate_product_optional_branch_requirement(nil, nil, nil),
    do: {:error, "repo_uri and branch_name are required when product_name is omitted"}

  defp validate_product_optional_branch_requirement(nil, _repo_uri, _branch_name), do: :ok

  defp validate_product_optional_branch_requirement(_product_name, _repo_uri, _branch_name),
    do: :ok

  defp build_implementation_entries(implementations) do
    Enum.map(implementations, fn implementation ->
      %{
        implementation_name: implementation.name,
        implementation_id: implementation.id,
        product_name: implementation.product.name
      }
    end)
  end

  defp optional_string(params, key) do
    case Map.get(params, key) do
      nil ->
        {:ok, nil}

      value when is_binary(value) ->
        trimmed = String.trim(value)
        if trimmed == "", do: {:error, "#{key} cannot be blank"}, else: {:ok, trimmed}

      value ->
        {:ok, to_string(value)}
    end
  end
end
