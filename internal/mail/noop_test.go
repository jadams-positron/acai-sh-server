package mail_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/mail"
)

func TestNoop_LogsMagicLinkURLAtInfo(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	n := mail.NewNoop(log)
	args := mail.MagicLinkArgs{
		To:        "alice@example.com",
		URL:       "https://acai.test/users/log-in/abc123",
		FromEmail: "no-reply@acai.sh",
		FromName:  "Acai",
	}
	if err := n.SendMagicLink(context.Background(), args); err != nil {
		t.Fatalf("SendMagicLink() unexpected error: %v", err)
	}

	var entry map[string]any
	if err := json.NewDecoder(&buf).Decode(&entry); err != nil {
		t.Fatalf("decode log line: %v", err)
	}

	if got, want := entry["level"], "INFO"; got != want {
		t.Errorf("level = %q, want %q", got, want)
	}
	if entry["to"] != args.To {
		t.Errorf("to = %v, want %q", entry["to"], args.To)
	}
	if entry["url"] != args.URL {
		t.Errorf("url = %v, want %q", entry["url"], args.URL)
	}
}
