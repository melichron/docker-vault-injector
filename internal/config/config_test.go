package config

import (
	"testing"
	"time"
)

func TestLoadRequiresExplicitAppRoleAuthPath(t *testing.T) {
	t.Setenv("VAULT_AUTH_METHOD", "approle")
	t.Setenv("VAULT_APPROLE_AUTH_PATH", "")
	if _, err := Load(); err == nil {
		t.Fatal("AppRole configuration without VAULT_APPROLE_AUTH_PATH should fail")
	}
}

func TestLoadAllowsExplicitTokenFallback(t *testing.T) {
	t.Setenv("VAULT_AUTH_METHOD", "token")
	t.Setenv("VAULT_APPROLE_AUTH_PATH", "")
	configuration, err := Load()
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}
	if configuration.VaultAuth.Method != "token" {
		t.Fatalf("auth method = %q", configuration.VaultAuth.Method)
	}
}

func TestStatusFileFromEnvironment(t *testing.T) {
	t.Setenv("INJECTOR_STATUS_FILE", "/run/injector/status.json")
	if got := StatusFileFromEnvironment(); got != "/run/injector/status.json" {
		t.Fatalf("status file = %q", got)
	}

	t.Setenv("INJECTOR_STATUS_FILE", "")
	if got := StatusFileFromEnvironment(); got != DefaultStatusFile {
		t.Fatalf("default status file = %q", got)
	}
}

func TestHealthMaxAgeFromEnvironment(t *testing.T) {
	t.Setenv("INJECTOR_HEALTH_MAX_AGE", "3m")
	if got, err := HealthMaxAgeFromEnvironment(); err != nil || got != 3*time.Minute {
		t.Fatalf("health max age = %s, err = %v", got, err)
	}

	t.Setenv("INJECTOR_HEALTH_MAX_AGE", "")
	if got, err := HealthMaxAgeFromEnvironment(); err != nil || got != DefaultHealthMaxAge {
		t.Fatalf("default health max age = %s, err = %v", got, err)
	}
}

func TestLoadRejectsRetryMaximumBelowInitialInterval(t *testing.T) {
	t.Setenv("VAULT_APPROLE_AUTH_PATH", "auth/swarm")
	t.Setenv("INJECTOR_EVENT_RETRY_INTERVAL", "10s")
	t.Setenv("INJECTOR_EVENT_RETRY_MAX_INTERVAL", "5s")
	if _, err := Load(); err == nil {
		t.Fatal("expected event retry maximum validation error")
	}
}
