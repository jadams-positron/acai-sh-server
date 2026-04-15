defmodule Acai.RepoTest do
  use Acai.DataCase, async: true

  defp index_defs(table_name) do
    Ecto.Adapters.SQL.query!(
      Repo,
      """
      SELECT indexdef
      FROM pg_indexes
      WHERE schemaname = current_schema() AND tablename = $1
      """,
      [table_name]
    ).rows
    |> Enum.map(fn [indexdef] -> indexdef end)
  end

  test "data-model.TOKENS.10-1 creates an index on access_tokens.team_id" do
    assert Enum.any?(index_defs("access_tokens"), &String.contains?(&1, "(team_id)"))
  end

  test "data-model.ROLES.2-1 creates an index on user_team_roles.user_id" do
    assert Enum.any?(index_defs("user_team_roles"), &String.contains?(&1, "(user_id)"))
  end

  test "data-model.IMPLS.6-1 creates an index on implementations.team_id" do
    assert Enum.any?(index_defs("implementations"), &String.contains?(&1, "(team_id)"))
  end

  test "data-model.TRACKED_BRANCHES.2-1 creates an index on tracked_branches.branch_id" do
    assert Enum.any?(index_defs("tracked_branches"), &String.contains?(&1, "(branch_id)"))
  end
end
