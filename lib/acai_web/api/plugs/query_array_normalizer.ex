defmodule AcaiWeb.Api.Plugs.QueryArrayNormalizer do
  @moduledoc """
  Normalizes repeated bare query params into arrays for OpenAPI validation.

  See feature-context.REQUEST.7-1 and implementation-features.REQUEST.3-1.
  """

  alias AcaiWeb.Api.Operations

  @array_query_keys_by_endpoint %{
    feature_context: ["statuses"],
    implementation_features: ["statuses"]
  }

  @spec init(keyword()) :: keyword()
  def init(opts), do: opts

  @spec call(Plug.Conn.t(), keyword()) :: Plug.Conn.t()
  def call(conn, _opts) do
    endpoint = Operations.endpoint_key(conn)

    case Map.get(@array_query_keys_by_endpoint, endpoint, []) do
      [] ->
        conn

      keys ->
        normalize_query_arrays(conn, keys)
    end
  end

  defp normalize_query_arrays(conn, keys) do
    updates =
      Enum.reduce(keys, %{}, fn key, acc ->
        case collect_query_values(conn.query_string, key) do
          [] -> acc
          values -> Map.put(acc, key, values)
        end
      end)

    if map_size(updates) == 0 do
      conn
    else
      %{
        conn
        | query_params: Map.merge(conn.query_params, updates),
          params: Map.merge(conn.params, updates)
      }
    end
  end

  defp collect_query_values(query_string, key) do
    query_string
    |> String.split("&", trim: true)
    |> Enum.reduce([], fn pair, acc ->
      case String.split(pair, "=", parts: 2) do
        [raw_key, raw_value] ->
          if URI.decode_www_form(raw_key) == key do
            [URI.decode_www_form(raw_value) | acc]
          else
            acc
          end

        [raw_key] ->
          if URI.decode_www_form(raw_key) == key do
            ["" | acc]
          else
            acc
          end

        _other ->
          acc
      end
    end)
    |> Enum.reverse()
  end
end
