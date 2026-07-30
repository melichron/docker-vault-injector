package labels

import (
	"reflect"
	"testing"
)

func TestParseConfig(t *testing.T) {
	configuration, err := ParseConfig(map[string]string{
		EnabledLabel: "true",
		SecretsLabel: `{
            "database": {
                "mount": "kv",
                "path": "apps/api/database",
                "env": {"DB_USER": "username", "DB_PASSWORD": "password"}
            }
        }`,
	})
	if err != nil {
		t.Fatalf("ParseConfig returned an error: %v", err)
	}
	if !configuration.Enabled {
		t.Fatal("expected configuration to be enabled")
	}
	want := []string{"DB_PASSWORD", "DB_USER"}
	if got := configuration.DesiredEnvironmentNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("environment names = %v, want %v", got, want)
	}
}

func TestParseConfigRejectsDuplicateEnvironmentOwnership(t *testing.T) {
	_, err := ParseConfig(map[string]string{
		EnabledLabel: "true",
		SecretsLabel: `{
            "one": {"mount":"kv", "path":"one", "env":{"TOKEN":"token"}},
            "two": {"mount":"kv", "path":"two", "env":{"TOKEN":"other"}}
        }`,
	})
	if err == nil {
		t.Fatal("expected duplicate environment mapping to fail")
	}
}

func TestDisabledConfigDoesNotRequireSecrets(t *testing.T) {
	configuration, err := ParseConfig(map[string]string{EnabledLabel: "false"})
	if err != nil {
		t.Fatalf("ParseConfig returned an error: %v", err)
	}
	if configuration.Enabled {
		t.Fatal("expected configuration to be disabled")
	}
}
