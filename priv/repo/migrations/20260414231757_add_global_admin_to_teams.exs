defmodule Acai.Repo.Migrations.AddGlobalAdminToTeams do
  use Ecto.Migration

  def change do
    alter table(:teams) do
      # data-model.TEAMS.3
      # data-model.TEAMS.3-1
      add :global_admin, :boolean, null: false, default: false
    end
  end
end
