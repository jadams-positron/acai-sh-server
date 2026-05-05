// Package config loads runtime configuration from environment variables.
//
// Fields are populated by Load and alphabetically sorted within each section.
// P0 adds LogLevel; P1a adds DatabasePath and HTTPPort.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config is the validated runtime configuration. All fields are populated by Load.
type Config struct {
	// DatabasePath is the path to the SQLite database file. Default: "./acai.db".
	DatabasePath string

	// HTTPPort is the TCP port the HTTP server listens on. Range [1, 65535]. Default: 4000.
	HTTPPort int

	// LogLevel is one of "debug", "info", "warn", "error". Default: "info".
	LogLevel string
}

// Load reads configuration from environment variables and validates it.
// Returns an error if any value fails validation.
func Load() (*Config, error) {
	cfg := &Config{
		DatabasePath: getenvDefault("DATABASE_PATH", "./acai.db"),
		LogLevel:     getenvDefault("LOG_LEVEL", "info"),
	}

	// HTTP_PORT
	portStr := getenvDefault("HTTP_PORT", "4000")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("config: invalid HTTP_PORT %q: not an integer", portStr)
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("config: invalid HTTP_PORT %d: must be in range [1, 65535]", port)
	}
	cfg.HTTPPort = port

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
