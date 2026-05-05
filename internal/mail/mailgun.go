package mail

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mailgun/mailgun-go/v4"
)

// Mailgun is a Mailer implementation backed by the Mailgun HTTP API.
type Mailgun struct {
	mg  *mailgun.MailgunImpl
	log *slog.Logger
}

// NewMailgun constructs a Mailgun mailer. If baseURL is empty, mailgun-go
// retains its built-in default (https://api.mailgun.net/v3). Pass a non-empty
// baseURL to override the region (e.g. https://api.eu.mailgun.net/v3).
func NewMailgun(domain, apiKey, baseURL string, log *slog.Logger) *Mailgun {
	mg := mailgun.NewMailgun(domain, apiKey)
	if baseURL != "" {
		mg.SetAPIBase(baseURL)
	}
	return &Mailgun{mg: mg, log: log}
}

// SendMagicLink sends a magic-link login email via the Mailgun API.
func (m *Mailgun) SendMagicLink(ctx context.Context, args MagicLinkArgs) error {
	from := fmt.Sprintf("%s <%s>", args.FromName, args.FromEmail)
	subject := "Your magic login link"

	plain := fmt.Sprintf("Click this link to log in:\n\n%s\n\nThe link expires in 15 minutes.", args.URL)
	html := fmt.Sprintf(
		`<p>Click the button below to log in. The link expires in 15&nbsp;minutes.</p>`+
			`<p><a href="%s">Log in to Acai</a></p>`,
		args.URL,
	)

	msg := mailgun.NewMessage(from, subject, plain, args.To)
	msg.SetHTML(html)

	resp, id, err := m.mg.Send(ctx, msg)
	if err != nil {
		m.log.Error("mail.mailgun: send failed",
			slog.String("to", args.To),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("mail.mailgun: send: %w", err)
	}

	m.log.Info("mail.mailgun: sent magic link",
		slog.String("to", args.To),
		slog.String("mailgun_id", id),
		slog.String("mailgun_response", resp),
	)
	return nil
}
