defmodule AcaiWeb.HealthController do
  use AcaiWeb, :controller

  alias Acai.Repo

  def health(conn, _params) do
    version = Application.spec(:acai, :vsn) |> to_string()

    # Database connectivity check
    db_status = check_database()

    # VM metrics
    memory_mb = :erlang.memory(:total) / 1_048_576
    process_count = :erlang.system_info(:process_count)

    # Uptime calculation
    uptime_seconds = calculate_uptime()

    response = %{
      status: if(db_status == :ok, do: "ok", else: "degraded"),
      version: version,
      uptime_seconds: uptime_seconds,
      database: db_status,
      vm: %{
        memory_mb: Float.round(memory_mb, 2),
        process_count: process_count
      }
    }

    status_code = if db_status == :ok, do: :ok, else: :service_unavailable
    conn |> put_status(status_code) |> json(response)
  end

  defp check_database do
    case Repo.query("SELECT 1", [], timeout: 5_000) do
      {:ok, _} -> :ok
      {:error, _} -> :error
    end
  rescue
    _ -> :error
  end

  defp calculate_uptime do
    case Application.get_env(:acai, :start_time) do
      nil -> 0
      start_time -> System.system_time(:second) - start_time
    end
  end
end
