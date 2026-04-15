defmodule Acai.Repo.Migrations.MakeSpecsProductScoped do
  use Ecto.Migration

  def change do
    drop_if_exists unique_index(:specs, [:branch_id, :feature_name])

    create unique_index(:specs, [:branch_id, :product_id, :feature_name])
  end
end
