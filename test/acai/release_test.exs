defmodule Acai.ReleaseTest do
  use Acai.DataCase, async: false

  test "release helper exports production entrypoints" do
    assert {:module, Acai.Release} = Code.ensure_loaded(Acai.Release)
    assert function_exported?(Acai.Release, :migrate, 0)
    assert function_exported?(Acai.Release, :rollback, 2)
  end
end
