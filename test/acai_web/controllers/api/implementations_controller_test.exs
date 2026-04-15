defmodule AcaiWeb.Api.ImplementationsControllerTest do
  @moduledoc false

  use AcaiWeb.ConnCase, async: false

  import Acai.DataModelFixtures

  alias Acai.AccountsFixtures
  alias Acai.Implementations.{Branch, Implementation}
  alias Acai.Repo
  alias Acai.Products.Product
  alias Acai.Specs.{FeatureBranchRef, FeatureImplState, Spec}
  alias Acai.Teams
  alias Acai.Teams.AccessToken

  describe "GET /api/v1/implementations" do
    setup do
      user = AccountsFixtures.user_fixture()
      team = team_fixture()
      user_team_role_fixture(team, user, %{title: "owner"})

      {:ok, token} = Teams.generate_token(%{user: user}, team, %{name: "Read Token"})

      %{team: team, user: user, token: token}
    end

    # implementations.AUTH.1, implementations.ENDPOINT.2, implementations.RESPONSE.9
    test "returns 401 when authorization is missing", %{conn: conn} do
      conn = get(conn, "/api/v1/implementations", %{"product_name" => "api"})

      assert json_response(conn, 401)
      assert conn.resp_body =~ "Authorization header required"
    end

    # implementations.AUTH.1
    test "returns 401 when the token has been revoked", %{conn: conn, token: token} do
      {:ok, _token} = Teams.revoke_token(token)

      conn =
        conn
        |> put_req_header("authorization", "Bearer #{token.raw_token}")
        |> get("/api/v1/implementations", %{"product_name" => "api"})

      assert %{"errors" => %{"detail" => "Token has been revoked"}} = json_response(conn, 401)
    end

    # implementations.AUTH.1
    test "returns 401 when the token has expired", %{conn: conn, user: user, team: team} do
      raw_token = "at_" <> Base.url_encode64(:crypto.strong_rand_bytes(32), padding: false)
      token_hash = :crypto.hash(:sha256, raw_token) |> Base.encode16(case: :lower)
      token_prefix = String.slice(raw_token, 0, 10)
      past_date = DateTime.utc_now() |> DateTime.add(-1, :day)

      %AccessToken{
        id: Acai.UUIDv7.autogenerate(),
        name: "Expired Token",
        token_hash: token_hash,
        token_prefix: token_prefix,
        scopes: ["impls:read"],
        expires_at: DateTime.truncate(past_date, :second),
        team_id: team.id,
        user_id: user.id,
        inserted_at: DateTime.utc_now(:second),
        updated_at: DateTime.utc_now(:second)
      }
      |> Repo.insert!()

      conn =
        conn
        |> put_req_header("authorization", "Bearer #{raw_token}")
        |> get("/api/v1/implementations", %{"product_name" => "api"})

      assert %{"errors" => %{"detail" => "Token has expired"}} = json_response(conn, 401)
    end

    # implementations.RESPONSE.8, implementations.FILTERS.3, implementations.FILTERS.4
    test "returns 422 when branch filters are incomplete", %{conn: conn, token: token} do
      conn =
        conn
        |> put_req_header("authorization", "Bearer #{token.raw_token}")
        |> get("/api/v1/implementations", %{
          "product_name" => "api",
          "repo_uri" => "github.com/acai/api"
        })

      assert json_response(conn, 422)
      assert conn.resp_body =~ "branch_name is required when repo_uri is provided"
    end

    # implementations.AUTH.3, implementations.RESPONSE.9, implementations.FILTERS.5, implementations.FILTERS.6
    test "returns 403 when feature filtering is requested without specs scope", %{
      conn: conn,
      team: team,
      user: user
    } do
      {:ok, limited_token} =
        Teams.generate_token(%{user: user}, team, %{
          name: "Impl Read Only",
          scopes: ["impls:read"]
        })

      conn =
        conn
        |> put_req_header("authorization", "Bearer #{limited_token.raw_token}")
        |> get("/api/v1/implementations", %{
          "product_name" => "api",
          "feature_name" => "alpha"
        })

      assert json_response(conn, 403)
      assert conn.resp_body =~ "specs:read"
    end

    # implementations.AUTH.2, implementations.RESPONSE.10
    test "returns 403 when impls:read scope is missing", %{conn: conn, team: team, user: user} do
      {:ok, limited_token} =
        Teams.generate_token(%{user: user}, team, %{
          name: "Specs Read Only",
          scopes: ["specs:read"]
        })

      conn =
        conn
        |> put_req_header("authorization", "Bearer #{limited_token.raw_token}")
        |> get("/api/v1/implementations", %{"product_name" => "api"})

      assert json_response(conn, 403)
      assert conn.resp_body =~ "impls:read"
    end

    # implementations.REQUEST.1-1, implementations.VALIDATION.1-1, implementations.RESPONSE.8
    test "returns 422 when product_name is omitted without branch filters", %{
      conn: conn,
      token: token
    } do
      conn =
        conn
        |> put_req_header("authorization", "Bearer #{token.raw_token}")
        |> get("/api/v1/implementations", %{})

      assert json_response(conn, 422)

      assert conn.resp_body =~
               "repo_uri and branch_name are required when product_name is omitted"
    end

    # implementations.RESPONSE.1, implementations.RESPONSE.2, implementations.RESPONSE.4, implementations.RESPONSE.5, implementations.RESPONSE.6
    test "returns alphabetically sorted implementations for a product", %{
      conn: conn,
      token: token,
      team: team
    } do
      product = product_fixture(team, %{name: "api"})

      _zulu = implementation_fixture(product, %{name: "Zulu"})
      _alpha = implementation_fixture(product, %{name: "Alpha"})

      conn =
        conn
        |> put_req_header("authorization", "Bearer #{token.raw_token}")
        |> get("/api/v1/implementations", %{"product_name" => "api"})

      assert %{"data" => data} = json_response(conn, 200)
      assert data["product_name"] == "api"
      assert Enum.map(data["implementations"], & &1["implementation_name"]) == ["Alpha", "Zulu"]
      assert Enum.map(data["implementations"], & &1["product_name"]) == ["api", "api"]
    end

    # implementations.AUTH.4
    test "product-scoped lookup excludes same-name products from another team", %{
      conn: conn,
      token: token,
      team: team
    } do
      product = product_fixture(team, %{name: "api"})
      included = implementation_fixture(product, %{name: "Alpha"})

      other_team = team_fixture(%{name: "other-team"})
      other_product = product_fixture(other_team, %{name: "api"})
      _excluded = implementation_fixture(other_product, %{name: "Zulu"})

      conn =
        conn
        |> put_req_header("authorization", "Bearer #{token.raw_token}")
        |> get("/api/v1/implementations", %{"product_name" => "api"})

      assert %{"data" => data} = json_response(conn, 200)

      assert Enum.map(data["implementations"], &{&1["implementation_id"], &1["product_name"]}) ==
               [{included.id, "api"}]
    end

    # implementations.FILTERS.1, implementations.FILTERS.2, implementations.FILTERS.5, implementations.RESPONSE.3, implementations.RESPONSE.7
    test "filters by exact branch and feature availability", %{
      conn: conn,
      token: token,
      team: team
    } do
      product = product_fixture(team, %{name: "api"})

      branch_main = branch_fixture(team, %{repo_uri: "github.com/acai/api", branch_name: "main"})
      branch_dev = branch_fixture(team, %{repo_uri: "github.com/acai/api", branch_name: "dev"})

      impl_main = implementation_fixture(product, %{name: "Main", is_active: true})
      impl_dev = implementation_fixture(product, %{name: "Dev", is_active: true})

      tracked_branch_fixture(impl_main, %{branch: branch_main})
      tracked_branch_fixture(impl_dev, %{branch: branch_dev})

      feature_name = "lookup-feature"
      spec_fixture(product, %{feature_name: feature_name, branch: branch_main})

      conn =
        conn
        |> put_req_header("authorization", "Bearer #{token.raw_token}")
        |> get("/api/v1/implementations", %{
          "product_name" => "api",
          "repo_uri" => "github.com/acai/api",
          "branch_name" => "main",
          "feature_name" => feature_name
        })

      assert %{"data" => data} = json_response(conn, 200)
      assert data["repo_uri"] == "github.com/acai/api"
      assert data["branch_name"] == "main"

      assert [%{"implementation_name" => "Main", "product_name" => "api"}] =
               data["implementations"]
    end

    # implementations.FILTERS.5, implementations.FILTERS.6
    test "includes inherited feature matches without local specs", %{
      conn: conn,
      token: token,
      team: team
    } do
      product = product_fixture(team, %{name: "api"})

      parent = implementation_fixture(product, %{name: "Parent", is_active: true})

      child =
        implementation_fixture(product, %{
          name: "Child",
          is_active: true,
          parent_implementation_id: parent.id
        })

      branch = branch_fixture(team, %{repo_uri: "github.com/acai/api", branch_name: "main"})
      tracked_branch_fixture(parent, %{branch: branch})
      tracked_branch_fixture(child, %{branch: branch})

      feature_name = "inherited-feature"
      spec_fixture(product, %{feature_name: feature_name, branch: branch})

      conn =
        conn
        |> put_req_header("authorization", "Bearer #{token.raw_token}")
        |> get("/api/v1/implementations", %{
          "product_name" => "api",
          "feature_name" => feature_name
        })

      assert %{"data" => data} = json_response(conn, 200)
      assert Enum.map(data["implementations"], & &1["implementation_name"]) == ["Child", "Parent"]
    end

    # implementations.FILTERS.1-1, implementations.FILTERS.2, implementations.FILTERS.7, implementations.RESPONSE.3, implementations.RESPONSE.5, implementations.RESPONSE.6-1
    test "returns branch-scoped cross-product matches sorted by product_name then implementation_name",
         %{conn: conn, token: token, team: team} do
      product_a = product_fixture(team, %{name: "api"})
      product_b = product_fixture(team, %{name: "cli"})

      shared_branch =
        branch_fixture(team, %{repo_uri: "github.com/acai/shared", branch_name: "main"})

      impl_b = implementation_fixture(product_b, %{name: "Zulu"})
      impl_a1 = implementation_fixture(product_a, %{name: "Same"})
      impl_a2 = implementation_fixture(product_b, %{name: "Same"})
      impl_a0 = implementation_fixture(product_a, %{name: "Alpha"})

      tracked_branch_fixture(impl_b, %{branch: shared_branch})
      tracked_branch_fixture(impl_a1, %{branch: shared_branch})
      tracked_branch_fixture(impl_a2, %{branch: shared_branch})
      tracked_branch_fixture(impl_a0, %{branch: shared_branch})

      conn =
        conn
        |> put_req_header("authorization", "Bearer #{token.raw_token}")
        |> get("/api/v1/implementations", %{
          "repo_uri" => "github.com/acai/shared",
          "branch_name" => "main"
        })

      assert %{"data" => data} = json_response(conn, 200)
      refute Map.has_key?(data, "product_name")
      assert data["repo_uri"] == "github.com/acai/shared"
      assert data["branch_name"] == "main"

      assert Enum.map(data["implementations"], &{&1["product_name"], &1["implementation_name"]}) ==
               [{"api", "Alpha"}, {"api", "Same"}, {"cli", "Same"}, {"cli", "Zulu"}]
    end

    # implementations.AUTH.4
    test "branch-scoped lookup excludes overlapping branch matches from another team", %{
      conn: conn,
      token: token,
      team: team
    } do
      product = product_fixture(team, %{name: "api"})
      branch = branch_fixture(team, %{repo_uri: "github.com/acai/shared", branch_name: "main"})
      included = implementation_fixture(product, %{name: "Resolver"})
      tracked_branch_fixture(included, %{branch: branch})

      other_team = team_fixture(%{name: "other-branch-team"})
      other_product = product_fixture(other_team, %{name: "api"})

      other_branch =
        branch_fixture(other_team, %{repo_uri: "github.com/acai/shared", branch_name: "main"})

      _excluded =
        implementation_fixture(other_product, %{name: "Resolver"})
        |> tracked_branch_fixture(branch: other_branch)

      conn =
        conn
        |> put_req_header("authorization", "Bearer #{token.raw_token}")
        |> get("/api/v1/implementations", %{
          "repo_uri" => "github.com/acai/shared",
          "branch_name" => "main"
        })

      assert %{"data" => data} = json_response(conn, 200)

      assert Enum.map(data["implementations"], &{&1["implementation_id"], &1["product_name"]}) ==
               [{included.id, "api"}]
    end

    # implementations.REQUEST.4, implementations.REQUEST.4-note, implementations.FILTERS.5, implementations.FILTERS.6
    test "filters branch-scoped results by feature availability in each implementation product",
         %{conn: conn, token: token, team: team} do
      product_a = product_fixture(team, %{name: "api"})
      product_b = product_fixture(team, %{name: "cli"})

      shared_branch =
        branch_fixture(team, %{repo_uri: "github.com/acai/shared", branch_name: "dev"})

      impl_a = implementation_fixture(product_a, %{name: "Resolver"})
      impl_b = implementation_fixture(product_b, %{name: "Resolver"})

      tracked_branch_fixture(impl_a, %{branch: shared_branch})
      tracked_branch_fixture(impl_b, %{branch: shared_branch})

      spec_fixture(product_b, %{feature_name: "cross-product-feature", branch: shared_branch})

      conn =
        conn
        |> put_req_header("authorization", "Bearer #{token.raw_token}")
        |> get("/api/v1/implementations", %{
          "repo_uri" => "github.com/acai/shared",
          "branch_name" => "dev",
          "feature_name" => "cross-product-feature"
        })

      assert %{"data" => data} = json_response(conn, 200)

      assert Enum.map(data["implementations"], &{&1["product_name"], &1["implementation_name"]}) ==
               [{"cli", "Resolver"}]
    end

    # implementations.RESPONSE.7
    test "returns an empty list when nothing matches", %{conn: conn, token: token, team: team} do
      _product = product_fixture(team, %{name: "api"})

      conn =
        conn
        |> put_req_header("authorization", "Bearer #{token.raw_token}")
        |> get("/api/v1/implementations", %{
          "product_name" => "api",
          "feature_name" => "missing-feature"
        })

      assert %{"data" => data} = json_response(conn, 200)
      assert data["implementations"] == []
    end

    # implementations.VALIDATION.4
    test "does not mutate products, implementations, branches, specs, refs, or states", %{
      conn: conn,
      token: token,
      team: team
    } do
      product = product_fixture(team, %{name: "api"})
      implementation = implementation_fixture(product, %{name: "ReadOnly"})
      branch = branch_fixture(team, %{repo_uri: "github.com/acai/api", branch_name: "main"})
      tracked_branch_fixture(implementation, %{branch: branch})
      spec = spec_fixture(product, %{branch: branch, feature_name: "read-only-feature"})
      feature_branch_ref_fixture(branch, spec.feature_name)
      spec_impl_state_fixture(spec, implementation)

      counts_before = resource_counts()

      conn =
        conn
        |> put_req_header("authorization", "Bearer #{token.raw_token}")
        |> get("/api/v1/implementations", %{"product_name" => "api"})

      assert %{"data" => _data} = json_response(conn, 200)
      assert resource_counts() == counts_before
    end
  end

  defp resource_counts do
    %{
      products: Repo.aggregate(Product, :count),
      implementations: Repo.aggregate(Implementation, :count),
      branches: Repo.aggregate(Branch, :count),
      specs: Repo.aggregate(Spec, :count),
      refs: Repo.aggregate(FeatureBranchRef, :count),
      states: Repo.aggregate(FeatureImplState, :count)
    }
  end
end
