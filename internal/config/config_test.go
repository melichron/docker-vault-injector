package config

import "testing"

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
