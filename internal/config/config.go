// Package config loads process-level configuration. Service-specific
// configuration belongs in Swarm labels and is parsed by internal/labels.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

type Config struct {
	PollInterval       time.Duration
	ReconcileTimeout   time.Duration
	EventRetryInterval time.Duration
	VaultTokenFile     string
	LogLevel           slog.Level
}

func Load() (Config, error) {
	pollInterval, err := durationFromEnvironment("INJECTOR_POLL_INTERVAL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	reconcileTimeout, err := durationFromEnvironment("INJECTOR_RECONCILE_TIMEOUT", 20*time.Second)
	if err != nil {
		return Config{}, err
	}
	eventRetryInterval, err := durationFromEnvironment("INJECTOR_EVENT_RETRY_INTERVAL", 5*time.Second)
	if err != nil {
		return Config{}, err
	}

	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("INJECTOR_LOG_LEVEL")) {
	case "", "info":
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return Config{}, fmt.Errorf("INJECTOR_LOG_LEVEL must be debug, info, warn, or error")
	}

	if pollInterval <= 0 || reconcileTimeout <= 0 || eventRetryInterval <= 0 {
		return Config{}, fmt.Errorf("all duration settings must be greater than zero")
	}

	return Config{
		PollInterval:       pollInterval,
		ReconcileTimeout:   reconcileTimeout,
		EventRetryInterval: eventRetryInterval,
		VaultTokenFile:     os.Getenv("VAULT_TOKEN_FILE"),
		LogLevel:           level,
	}, nil
}

func durationFromEnvironment(name string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return value, nil
}
