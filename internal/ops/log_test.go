package ops_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/jadams-positron/acai-sh-server/internal/config"
	"github.com/jadams-positron/acai-sh-server/internal/ops"
)

func TestSetupLogger_EmitsValidJSON(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.Config{LogLevel: "info"}
	logger := ops.SetupLogger(cfg, &buf)

	logger.Info("hello", slog.String("foo", "bar"))

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatalf("logger emitted nothing; buf=%q", buf.String())
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		t.Fatalf("logger output is not valid JSON: %v\nline=%q", err, line)
	}
	if parsed["msg"] != "hello" {
		t.Errorf(`parsed["msg"] = %v, want "hello"`, parsed["msg"])
	}
	if parsed["foo"] != "bar" {
		t.Errorf(`parsed["foo"] = %v, want "bar"`, parsed["foo"])
	}
	if _, ok := parsed["time"]; !ok {
		t.Errorf(`parsed missing "time" key; got %v`, parsed)
	}
}

func TestSetupLogger_RespectsLogLevel(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.Config{LogLevel: "warn"}
	logger := ops.SetupLogger(cfg, &buf)

	logger.Info("should-be-filtered")
	logger.Warn("should-be-emitted")

	out := buf.String()
	if strings.Contains(out, "should-be-filtered") {
		t.Errorf("info-level message leaked through warn level: %q", out)
	}
	if !strings.Contains(out, "should-be-emitted") {
		t.Errorf("warn-level message was not emitted: %q", out)
	}
}

func TestSetupLogger_UsesJSONHandlerNotText(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.Config{LogLevel: "info"}
	logger := ops.SetupLogger(cfg, &buf)

	logger.Info("probe")
	out := strings.TrimSpace(buf.String())

	if !strings.HasPrefix(out, "{") {
		t.Errorf("logger output is not JSON-formatted (must use slog.NewJSONHandler): %q", out)
	}
}
