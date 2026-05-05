// Package config loads runtime configuration from environment variables.
//
// In Phase 0 the only field is LogLevel. Subsequent phases extend this struct
// (database path, http port, mailer settings, etc.) — keep additions
// alphabetically sorted within each section.
package config

import (
	"fmt"
	"os"
)

// Config is the validated runtime configuration. All fields are populated by Load.
type Config struct {
	// LogLevel is one of "debug", "info", "warn", "error". Default: "info".
	LogLevel string
}

// Load reads configuration from environment variables and validates it.
// Returns an error if any value fails validation.
func Load() (*Config, error) {
	cfg := &Config{
		LogLevel: getenvDefault("LOG_LEVEL", "info"),
	}
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
		// ok
	default:
		return nil, fmt.Errorf("config: invalid LOG_LEVEL %q (allowed: debug, info, warn, error)", cfg.LogLevel)
	}
	return cfg, nil
}

func getenvDefault(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
