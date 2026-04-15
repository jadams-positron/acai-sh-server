defmodule Acai.Repo.Migrations.AddMissingFkIndexes do
  use Ecto.Migration

  def change do
    # data-model.TOKENS.10-1
    create index(:access_tokens, [:team_id])

    # data-model.ROLES.2-1
    create index(:user_team_roles, [:user_id])

    # data-model.IMPLS.6-1
    create index(:implementations, [:team_id])

    # data-model.TRACKED_BRANCHES.2-1
    create index(:tracked_branches, [:branch_id])
  end
end
