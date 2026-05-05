package mail_test

import (
	"log/slog"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/config"
	"github.com/jadams-positron/acai-sh-server/internal/mail"
)

func TestNewFromConfig_NoopWhenMailNoopTrue(t *testing.T) {
	cfg := &config.Config{
		MailNoop: true,
	}
	log := slog.Default()

	m := mail.NewFromConfig(cfg, log)
	if _, ok := m.(*mail.Noop); !ok {
		t.Errorf("NewFromConfig() with MailNoop=true returned %T, want *mail.Noop", m)
	}
}

func TestNewFromConfig_MailgunWhenMailNoopFalse(t *testing.T) {
	cfg := &config.Config{
		MailNoop:       false,
		MailgunDomain:  "mg.example.com",
		MailgunAPIKey:  "key-test",
		MailgunBaseURL: "https://api.mailgun.net/v3",
	}
	log := slog.Default()

	m := mail.NewFromConfig(cfg, log)
	if _, ok := m.(*mail.Mailgun); !ok {
		t.Errorf("NewFromConfig() with MailNoop=false returned %T, want *mail.Mailgun", m)
	}
}
