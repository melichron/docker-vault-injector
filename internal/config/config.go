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

const (
	DefaultStatusFile   = "/tmp/docker-vault-injector-status.json"
	DefaultHealthMaxAge = 80 * time.Second
)

type Config struct {
	PollInterval       time.Duration
	ReconcileTimeout   time.Duration
	EventRetryInterval time.Duration
	EventRetryMaximum  time.Duration
	HealthMaxAge       time.Duration
	StatusFile         string
	VaultAuth          VaultAuthConfig
	LogLevel           slog.Level
}

// VaultAuthConfig contains authentication settings only. The Vault address,
// namespace and TLS settings continue to use the standard VAULT_* variables
// understood by HashiCorp's official Go client.
type VaultAuthConfig struct {
	Method              string
	AppRoleAuthPath     string
	AppRoleRoleID       string
	AppRoleRoleIDFile   string
	AppRoleSecretID     string
	AppRoleSecretIDFile string
	Token               string
	TokenFile           string
	TokenCheckInterval  time.Duration
	AuthRetryInterval   time.Duration
	AuthRetryMaximum    time.Duration
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
	eventRetryMaximum, err := durationFromEnvironment("INJECTOR_EVENT_RETRY_MAX_INTERVAL", time.Minute)
	if err != nil {
		return Config{}, err
	}
	healthMaxAge, err := HealthMaxAgeFromEnvironment()
	if err != nil {
		return Config{}, err
	}
	tokenCheckInterval, err := durationFromEnvironment("VAULT_TOKEN_CHECK_INTERVAL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	authRetryInterval, err := durationFromEnvironment("VAULT_AUTH_RETRY_INTERVAL", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	authRetryMaximum, err := durationFromEnvironment("VAULT_AUTH_RETRY_MAX_INTERVAL", time.Minute)
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

	if pollInterval <= 0 || reconcileTimeout <= 0 || eventRetryInterval <= 0 ||
		eventRetryMaximum <= 0 || healthMaxAge <= 0 || tokenCheckInterval <= 0 ||
		authRetryInterval <= 0 || authRetryMaximum <= 0 {
		return Config{}, fmt.Errorf("all duration settings must be greater than zero")
	}
	if eventRetryMaximum < eventRetryInterval {
		return Config{}, fmt.Errorf("INJECTOR_EVENT_RETRY_MAX_INTERVAL must be greater than or equal to INJECTOR_EVENT_RETRY_INTERVAL")
	}
	if authRetryMaximum < authRetryInterval {
		return Config{}, fmt.Errorf("VAULT_AUTH_RETRY_MAX_INTERVAL must be greater than or equal to VAULT_AUTH_RETRY_INTERVAL")
	}

	authMethod := strings.ToLower(strings.TrimSpace(os.Getenv("VAULT_AUTH_METHOD")))
	if authMethod == "" {
		authMethod = "approle"
	}
	if authMethod != "approle" && authMethod != "token" {
		return Config{}, fmt.Errorf("VAULT_AUTH_METHOD must be approle or token")
	}
	if authMethod == "approle" && strings.TrimSpace(os.Getenv("VAULT_APPROLE_AUTH_PATH")) == "" {
		return Config{}, fmt.Errorf("VAULT_APPROLE_AUTH_PATH is required for AppRole authentication")
	}

	return Config{
		PollInterval:       pollInterval,
		ReconcileTimeout:   reconcileTimeout,
		EventRetryInterval: eventRetryInterval,
		EventRetryMaximum:  eventRetryMaximum,
		HealthMaxAge:       healthMaxAge,
		StatusFile:         StatusFileFromEnvironment(),
		VaultAuth: VaultAuthConfig{
			Method:              authMethod,
			AppRoleAuthPath:     os.Getenv("VAULT_APPROLE_AUTH_PATH"),
			AppRoleRoleID:       os.Getenv("VAULT_APPROLE_ROLE_ID"),
			AppRoleRoleIDFile:   os.Getenv("VAULT_APPROLE_ROLE_ID_FILE"),
			AppRoleSecretID:     os.Getenv("VAULT_APPROLE_SECRET_ID"),
			AppRoleSecretIDFile: os.Getenv("VAULT_APPROLE_SECRET_ID_FILE"),
			Token:               os.Getenv("VAULT_TOKEN"),
			TokenFile:           os.Getenv("VAULT_TOKEN_FILE"),
			TokenCheckInterval:  tokenCheckInterval,
			AuthRetryInterval:   authRetryInterval,
			AuthRetryMaximum:    authRetryMaximum,
		},
		LogLevel: level,
	}, nil
}

// StatusFileFromEnvironment is intentionally independent from Load so the
// status subcommands can read the running controller's snapshot without
// requiring Docker or Vault authentication settings.
func StatusFileFromEnvironment() string {
	if value := strings.TrimSpace(os.Getenv("INJECTOR_STATUS_FILE")); value != "" {
		return value
	}
	return DefaultStatusFile
}

// HealthMaxAgeFromEnvironment remains independent from Load for the same
// reason as StatusFileFromEnvironment: the health subprocess must not require
// Vault credentials. The default covers two normal polling periods plus one
// reconciliation timeout.
func HealthMaxAgeFromEnvironment() (time.Duration, error) {
	return durationFromEnvironment("INJECTOR_HEALTH_MAX_AGE", DefaultHealthMaxAge)
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
