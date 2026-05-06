// Package ops contains operational primitives: structured logging, health
// checks, telemetry. Phase 0 wires only the logger.
package ops

import (
	"io"
	"log/slog"

	"github.com/jadams-positron/acai-sh-server/internal/config"
)

// SetupLogger returns a *slog.Logger configured with slog.NewJSONHandler
// writing to w. The logger's level is taken from cfg.LogLevel.
//
// Per spec §2: JSON output is mandatory in all environments. Do not introduce
// a TextHandler path even for "developer convenience" — humans read JSON via
// `jq` and machines read it natively.
func SetupLogger(cfg *config.Config, w io.Writer) *slog.Logger {
	level := parseLevel(cfg.LogLevel)
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
	})
	return slog.New(handler)
}

// SetupLoggerMinimal returns a *slog.Logger writing JSON at info level to w,
// without requiring a full Config. Used by utility subcommands (litestream
// status, restore) that don't load the full app config.
func SetupLoggerMinimal(w io.Writer) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(handler)
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
