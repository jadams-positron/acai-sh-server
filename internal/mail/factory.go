package mail

import (
	"log/slog"

	"github.com/jadams-positron/acai-sh-server/internal/config"
)

// NewFromConfig constructs the appropriate Mailer based on the given Config.
// When cfg.MailNoop is true, a Noop mailer is returned (no emails sent).
// Otherwise, a live Mailgun mailer is returned.
func NewFromConfig(cfg *config.Config, log *slog.Logger) Mailer {
	if cfg.MailNoop {
		return NewNoop(log)
	}
	return NewMailgun(cfg.MailgunDomain, cfg.MailgunAPIKey, cfg.MailgunBaseURL, log)
}
