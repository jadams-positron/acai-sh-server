package config_test

import (
	"testing"

	"github.com/acai-sh/server/internal/config"
)

func TestLoad_DefaultsWhenEnvMissing(t *testing.T) {
	t.Setenv("LOG_LEVEL", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("Load() LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

func TestLoad_HonorsLogLevelEnv(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("Load() LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}

func TestLoad_RejectsInvalidLogLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "verbose")

	_, err := config.Load()
	if err == nil {
		t.Fatalf("Load() with LOG_LEVEL=verbose should have errored, got nil")
	}
}

func TestLoad_DefaultsForDatabasePathAndHTTPPort(t *testing.T) {
	t.Setenv("DATABASE_PATH", "")
	t.Setenv("HTTP_PORT", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.DatabasePath != "./acai.db" {
		t.Errorf("Load() DatabasePath = %q, want %q", cfg.DatabasePath, "./acai.db")
	}
	if cfg.HTTPPort != 4000 {
		t.Errorf("Load() HTTPPort = %d, want %d", cfg.HTTPPort, 4000)
	}
}

func TestLoad_HonorsDatabasePathAndHTTPPort(t *testing.T) {
	t.Setenv("DATABASE_PATH", "/data/test.db")
	t.Setenv("HTTP_PORT", "8080")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.DatabasePath != "/data/test.db" {
		t.Errorf("Load() DatabasePath = %q, want %q", cfg.DatabasePath, "/data/test.db")
	}
	if cfg.HTTPPort != 8080 {
		t.Errorf("Load() HTTPPort = %d, want %d", cfg.HTTPPort, 8080)
	}
}

func TestLoad_RejectsInvalidHTTPPort(t *testing.T) {
	t.Setenv("HTTP_PORT", "not-a-number")

	_, err := config.Load()
	if err == nil {
		t.Fatalf("Load() with HTTP_PORT=not-a-number should have errored, got nil")
	}
}

func TestLoad_RejectsOutOfRangeHTTPPort(t *testing.T) {
	t.Setenv("HTTP_PORT", "99999")

	_, err := config.Load()
	if err == nil {
		t.Fatalf("Load() with HTTP_PORT=99999 should have errored, got nil")
	}
}
