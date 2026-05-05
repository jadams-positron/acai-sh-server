// Package mail owns transactional email dispatch. It defines the Mailer
// interface and the concrete implementations (Noop for development/testing,
// and eventually a live SMTP/API sender).
package mail

import "context"

// Mailer is the contract for sending transactional emails.
type Mailer interface {
	SendMagicLink(ctx context.Context, args MagicLinkArgs) error
}

// MagicLinkArgs carries the parameters for a magic-link login email.
type MagicLinkArgs struct {
	To        string
	URL       string
	FromEmail string
	FromName  string
}
