package mail

import (
	"context"
	"log/slog"
)

// Noop is a Mailer implementation that logs emails instead of sending them.
// It is suitable for development and testing.
type Noop struct {
	log *slog.Logger
}

// NewNoop constructs a Noop mailer that writes log entries to the given logger.
func NewNoop(log *slog.Logger) *Noop {
	return &Noop{log: log}
}

// SendMagicLink logs the magic-link details at Info level instead of sending
// an email. It always returns nil.
func (n *Noop) SendMagicLink(_ context.Context, args MagicLinkArgs) error {
	n.log.Info("mail.noop: magic link",
		slog.String("to", args.To),
		slog.String("url", args.URL),
		slog.String("from_email", args.FromEmail),
		slog.String("from_name", args.FromName),
	)
	return nil
}
