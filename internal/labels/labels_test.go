package labels

import "testing"

func TestParseFlatSourceLabels(t *testing.T) {
	configuration, err := ParseConfig(map[string]string{
		EnabledLabel:                               "true",
		SecretsPrefix + "database.name":            "db-vars",
		SecretsPrefix + "database.kv":              "docker-db-secrets",
		SecretsPrefix + "database.vault-path":      "stage/backend/db-conns",
		SecretsPrefix + "database.env.DB_USER":     "username",
		SecretsPrefix + "database.env.DB_PASSWORD": "password",
	})
	if err != nil {
		t.Fatalf("ParseConfig returned an error: %v", err)
	}
	source := configuration.Secrets["database"]
	if source.Name != "db-vars" || source.Mount != "docker-db-secrets" || source.Path != "stage/backend/db-conns" {
		t.Fatalf("unexpected source: %#v", source)
	}
	if source.Env["DB_USER"] != "username" || source.Env["DB_PASSWORD"] != "password" {
		t.Fatalf("unexpected mapping: %#v", source.Env)
	}
}

func TestSourceWithoutEnvUsesAutomaticImport(t *testing.T) {
	configuration, err := ParseConfig(map[string]string{
		EnabledLabel:                        "true",
		SecretsPrefix + "common.name":       "common-vars",
		SecretsPrefix + "common.kv":         "docker-swarm-secrets",
		SecretsPrefix + "common.vault-path": "stage/backend/common",
	})
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Secrets["common"].Env != nil {
		t.Fatal("an omitted env mapping must remain nil and select automatic import")
	}
}

func TestParseConfigRejectsDuplicateExplicitEnvironmentOwnership(t *testing.T) {
	_, err := ParseConfig(map[string]string{
		EnabledLabel:                     "true",
		SecretsPrefix + "one.name":       "one",
		SecretsPrefix + "one.kv":         "kv",
		SecretsPrefix + "one.vault-path": "one",
		SecretsPrefix + "one.env.TOKEN":  "token",
		SecretsPrefix + "two.name":       "two",
		SecretsPrefix + "two.kv":         "kv",
		SecretsPrefix + "two.vault-path": "two",
		SecretsPrefix + "two.env.TOKEN":  "other",
	})
	if err == nil {
		t.Fatal("expected duplicate environment mapping to fail")
	}
}

func TestParseConfigDoesNotEnforceShellVariableNaming(t *testing.T) {
	_, err := ParseConfig(map[string]string{
		EnabledLabel:                                "true",
		SecretsPrefix + "source.name":               "source",
		SecretsPrefix + "source.kv":                 "kv",
		SecretsPrefix + "source.vault-path":         "path",
		SecretsPrefix + "source.env.name-with-dash": "value",
	})
	if err != nil {
		t.Fatalf("the controller should leave environment naming policy to the operator: %v", err)
	}
}

func TestConfigHashChangesWithSourceConfiguration(t *testing.T) {
	first, err := ParseConfig(map[string]string{
		EnabledLabel:                        "true",
		SecretsPrefix + "source.name":       "source",
		SecretsPrefix + "source.kv":         "kv",
		SecretsPrefix + "source.vault-path": "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseConfig(map[string]string{
		EnabledLabel:                        "true",
		SecretsPrefix + "source.name":       "source",
		SecretsPrefix + "source.kv":         "kv",
		SecretsPrefix + "source.vault-path": "second",
	})
	if err != nil {
		t.Fatal(err)
	}

	firstHash, err := ConfigHash(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := ConfigHash(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash == secondHash {
		t.Fatal("different source configurations must not have the same state hash")
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
