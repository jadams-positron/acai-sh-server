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
