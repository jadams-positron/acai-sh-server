defmodule Acai.Accounts.UserNotifier do
  import Swoosh.Email

  alias Acai.Accounts.User
  alias Acai.Mailer

  defp deliver(recipient, subject, text_body, html_body) do
    from_name = Application.get_env(:acai, :mail_from_name, "Acai")
    from_email = Application.get_env(:acai, :mail_from_email, "noreply@example.com")

    email =
      new()
      |> to(recipient)
      |> from({from_name, from_email})
      |> subject(subject)
      |> text_body(text_body)
      |> html_body(html_body)

    with {:ok, _metadata} <- Mailer.deliver(email) do
      {:ok, email}
    end
  end

  defp deliver_link_email(recipient, subject, heading, intro, url, note) do
    deliver(
      recipient,
      subject,
      """
      #{heading}

      #{intro}

      #{url}

      #{note}

      Acai
      """,
      html_link_email(heading, intro, url, note)
    )
  end

  defp deliver_note_email(recipient, subject, heading, body, note) do
    deliver(
      recipient,
      subject,
      """
      #{heading}

      #{body}

      #{note}

      Acai
      """,
      html_note_email(heading, body, note)
    )
  end

  defp html_link_email(heading, intro, url, note) do
    """
    <!doctype html>
    <html>
      <body style="margin:0;padding:0;background:#f8f7fb;color:#1f2937;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;">
        <div style="max-width:560px;margin:0 auto;padding:32px 20px;">
          <div style="padding:24px;border:1px solid #e5e7eb;border-radius:8px;background:#fff;">
            <p style="margin:0 0 12px;font-size:12px;font-weight:600;letter-spacing:.08em;text-transform:uppercase;color:#930d7a;">Acai</p>
            <h1 style="margin:0 0 16px;font-size:20px;line-height:1.4;">#{escape_html(heading)}</h1>
            <p style="margin:0 0 20px;line-height:1.6;">#{escape_html(intro)}</p>
            <p style="margin:0 0 20px;">
              <a href="#{escape_html(url)}" style="display:inline-block;padding:12px 18px;border:1.5px solid #930d7a;border-radius:4px;background:#930d7a;color:#fff;text-decoration:none;font-weight:600;">Open link</a>
            </p>
            <p style="margin:0;color:#6b7280;line-height:1.6;">#{escape_html(note)}</p>
          </div>
        </div>
      </body>
    </html>
    """
  end

  defp html_note_email(heading, body, note) do
    """
    <!doctype html>
    <html>
      <body style="margin:0;padding:0;background:#f8f7fb;color:#1f2937;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;">
        <div style="max-width:560px;margin:0 auto;padding:32px 20px;">
          <div style="padding:24px;border:1px solid #e5e7eb;border-radius:8px;background:#fff;">
            <p style="margin:0 0 12px;font-size:12px;font-weight:600;letter-spacing:.08em;text-transform:uppercase;color:#930d7a;">Acai</p>
            <h1 style="margin:0 0 16px;font-size:20px;line-height:1.4;">#{escape_html(heading)}</h1>
            <p style="margin:0 0 20px;line-height:1.6;">#{escape_html(body)}</p>
            <p style="margin:0;color:#6b7280;line-height:1.6;">#{escape_html(note)}</p>
          </div>
        </div>
      </body>
    </html>
    """
  end

  defp escape_html(value) do
    value |> Phoenix.HTML.html_escape() |> Phoenix.HTML.safe_to_string()
  end

  @doc """
  Deliver instructions to update a user email.
  """
  def deliver_update_email_instructions(user, url) do
    deliver_link_email(
      user.email,
      "Confirm your new email",
      "Confirm your email address",
      "Use the link below to confirm your new email address.",
      url,
      "If you did not request this change, you can ignore this email."
    )
  end

  @doc """
  Deliver a simple notification to an existing user that they were added to a team.
  """
  # team-view.INVITE.3-3
  def deliver_team_added_notification(user, team_name) do
    deliver_note_email(
      user.email,
      "You've been added to #{team_name}",
      "Added to #{team_name}",
      "You now have access to the team #{team_name} in Acai.",
      "Log in to your account to continue."
    )
  end

  @doc """
  Deliver instructions to log in with a magic link.
  """
  def deliver_login_instructions(user, url) do
    case user do
      %User{confirmed_at: nil} -> deliver_confirmation_instructions(user, url)
      _ -> deliver_magic_link_instructions(user, url)
    end
  end

  defp deliver_magic_link_instructions(user, url) do
    deliver_link_email(
      user.email,
      "Your Acai sign-in link",
      "Sign in to Acai",
      "Use the link below to sign in to your account.",
      url,
      "If you did not request this email, you can ignore it."
    )
  end

  defp deliver_confirmation_instructions(user, url) do
    deliver_link_email(
      user.email,
      "Confirmation instructions for Acai",
      "Confirm your account",
      "Use the link below to activate your account.",
      url,
      "If you did not create this account, you can ignore this email."
    )
  end
end
