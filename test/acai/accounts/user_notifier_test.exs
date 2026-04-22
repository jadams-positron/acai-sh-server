defmodule Acai.Accounts.UserNotifierTest do
  use Acai.DataCase, async: false

  alias Acai.Accounts.UserNotifier

  import Acai.AccountsFixtures

  setup do
    previous_name = Application.get_env(:acai, :mail_from_name)
    previous_email = Application.get_env(:acai, :mail_from_email)

    Application.put_env(:acai, :mail_from_name, "Acai")
    Application.put_env(:acai, :mail_from_email, "noreply@example.com")

    on_exit(fn ->
      if is_nil(previous_name) do
        Application.delete_env(:acai, :mail_from_name)
      else
        Application.put_env(:acai, :mail_from_name, previous_name)
      end

      if is_nil(previous_email) do
        Application.delete_env(:acai, :mail_from_email)
      else
        Application.put_env(:acai, :mail_from_email, previous_email)
      end
    end)

    :ok
  end

  test "uses sender identity from app config" do
    Application.put_env(:acai, :mail_from_name, "Acai Team")
    Application.put_env(:acai, :mail_from_email, "hello@example.com")

    {:ok, email} =
      UserNotifier.deliver_update_email_instructions(user_fixture(), "https://example.com")

    assert email.from == {"Acai Team", "hello@example.com"}
  end

  test "renders branded login text and HTML" do
    user = user_fixture()
    url = "https://example.com/magic-link"

    {:ok, email} = UserNotifier.deliver_login_instructions(user, url)

    assert email.subject == "Your Acai sign-in link"
    assert String.contains?(email.text_body, "Sign in to Acai")
    assert String.contains?(email.text_body, url)
    assert String.contains?(email.html_body, "Acai")
    assert String.contains?(email.html_body, "background:#930d7a")
    assert String.contains?(email.html_body, "href=\"#{url}\"")
  end

  test "keeps confirmation copy concise" do
    user = unconfirmed_user_fixture()
    url = "https://example.com/confirm"

    {:ok, email} = UserNotifier.deliver_login_instructions(user, url)

    assert email.subject =~ "Confirmation instructions"
    assert String.contains?(email.text_body, "Confirm your account")
    assert String.contains?(email.text_body, url)
  end

  test "keeps team-added copy concise" do
    {:ok, email} = UserNotifier.deliver_team_added_notification(user_fixture(), "Platform")

    assert email.subject == "You've been added to Platform"
    assert String.contains?(email.text_body, "Platform")
    assert String.contains?(email.html_body, "Platform")
    assert String.contains?(email.html_body, "Log in to your account")
  end
end
