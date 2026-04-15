defmodule Acai.Teams.TeamTest do
  use Acai.DataCase, async: true

  alias Acai.Teams.Team

  describe "changeset/2" do
    # data-model.TEAMS.2
    test "valid with a URL-safe name" do
      cs = Team.changeset(%Team{}, %{name: "my-team_1"})
      assert cs.valid?
    end

    # data-model.TEAMS.2
    test "normalizes name to lowercase" do
      cs = Team.changeset(%Team{}, %{name: "MY-Team"})
      assert cs.valid?
      assert Ecto.Changeset.get_change(cs, :name) == "my-team"
    end

    test "invalid without a name" do
      cs = Team.changeset(%Team{}, %{})
      refute cs.valid?
      assert %{name: [_ | _]} = errors_on(cs)
    end

    # data-model.TEAMS.2-1
    test "invalid when name contains spaces" do
      cs = Team.changeset(%Team{}, %{name: "my team"})
      refute cs.valid?
    end

    test "invalid when name contains special characters" do
      cs = Team.changeset(%Team{}, %{name: "my@team"})
      refute cs.valid?
    end

    # data-model.TEAMS.1
    test "uses UUIDv7 primary key" do
      assert Team.__schema__(:primary_key) == [:id]
      assert Team.__schema__(:type, :id) == Acai.UUIDv7
    end

    # data-model.TEAMS.3
    # data-model.TEAMS.3-1
    test "exposes global_admin with a default of false" do
      assert :global_admin in Team.__schema__(:fields)
      assert Team.__schema__(:type, :global_admin) == :boolean
      assert %Team{}.global_admin == false
    end

    # data-model.TEAMS.3
    test "does not allow global_admin through the public changeset" do
      cs = Team.changeset(%Team{}, %{name: "admins", global_admin: true})
      assert cs.valid?
      assert Ecto.Changeset.get_change(cs, :global_admin) == nil
    end

    # data-model.TEAMS.3
    test "accepts global_admin in the trusted changeset" do
      cs = Team.trusted_changeset(%Team{}, %{name: "admins", global_admin: true})

      assert cs.valid?
      assert Ecto.Changeset.get_change(cs, :global_admin) == true
    end
  end

  describe "database constraints" do
    # data-model.TEAMS.2
    test "name must be unique (case-insensitive)" do
      import Acai.DataModelFixtures
      _team = team_fixture(%{name: "unique-team"})

      {:error, cs} =
        Team.changeset(%Team{}, %{name: "UNIQUE-TEAM"})
        |> Acai.Repo.insert()

      assert %{name: [_ | _]} = errors_on(cs)
    end

    # data-model.TEAMS.2-1
    test "name_url_safe check constraint fires for invalid chars" do
      {:error, cs} =
        %Team{}
        |> Team.changeset(%{name: "valid-name"})
        |> Ecto.Changeset.put_change(:name, "invalid name!")
        |> Acai.Repo.insert()

      assert cs.errors[:name] != nil
    end

    # data-model.TEAMS.3-1
    test "database default sets global_admin to false" do
      import Acai.DataModelFixtures

      team = team_fixture()
      assert team.global_admin == false
    end
  end
end
