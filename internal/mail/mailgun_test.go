package mail_test

import (
	"log/slog"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/mail"
)

func TestNewMailgun_ProducesUsableStruct(t *testing.T) {
	log := slog.Default()
	m := mail.NewMailgun("mg.example.com", "key-abc123", "https://api.mailgun.net/v3", log)
	if m == nil {
		t.Fatal("NewMailgun() returned nil")
	}
}

func TestNewMailgun_EmptyBaseURLLeavesDefault(t *testing.T) {
	log := slog.Default()
	// Should not panic; empty baseURL leaves mailgun-go's built-in default intact.
	m := mail.NewMailgun("mg.example.com", "key-abc123", "", log)
	if m == nil {
		t.Fatal("NewMailgun() with empty baseURL returned nil")
	}
}
