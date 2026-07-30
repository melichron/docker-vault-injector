// Package labels owns the public label contract between a Swarm service and
// docker-vault-injector.
//
// User configuration is deliberately spread across ordinary scalar labels.
// Large JSON/YAML block labels are awkward to template, merge and override in
// deployment systems. A source named "database" therefore looks like:
//
//	io.github.docker-vault-injector.secrets.database.name
//	io.github.docker-vault-injector.secrets.database.kv
//	io.github.docker-vault-injector.secrets.database.vault-path
package labels

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	Prefix = "io.github.docker-vault-injector"

	EnabledLabel         = Prefix + ".enabled"
	SecretsPrefix        = Prefix + ".secrets."
	AppliedVersionsLabel = Prefix + ".applied-versions"
	ManagedEnvLabel      = Prefix + ".managed-env"
	StateHashLabel       = Prefix + ".state-hash"
	ConfigHashLabel      = Prefix + ".config-hash"
)

// Config is the user-controlled portion of the service configuration.
type Config struct {
	Enabled bool
	Secrets map[string]SecretSource
}

// SecretSource describes one Vault KV v2 document. An empty Env map means
// "import every top-level scalar field under its original name". A non-empty
// map means "import only these explicit environment -> Vault field mappings".
type SecretSource struct {
	Name  string            `json:"name"`
	Mount string            `json:"mount"`
	Path  string            `json:"path"`
	Env   map[string]string `json:"env,omitempty"`
}

// ParseConfig groups flat labels by the source segment between `secrets.` and
// the property name. Unknown properties are rejected: a typo in `vault-path`
// should fail safely instead of silently reading the wrong configuration.
func ParseConfig(serviceLabels map[string]string) (Config, error) {
	result := Config{}

	rawEnabled, exists := serviceLabels[EnabledLabel]
	if !exists {
		return result, nil
	}

	enabled, err := strconv.ParseBool(rawEnabled)
	if err != nil {
		return Config{}, fmt.Errorf("%s must be true or false: %w", EnabledLabel, err)
	}
	result.Enabled = enabled
	if !enabled {
		return result, nil
	}

	result.Secrets = make(map[string]SecretSource)
	for labelName, value := range serviceLabels {
		if !strings.HasPrefix(labelName, SecretsPrefix) {
			continue
		}

		remainder := strings.TrimPrefix(labelName, SecretsPrefix)
		sourceID, property, found := strings.Cut(remainder, ".")
		if !found || sourceID == "" || property == "" {
			return Config{}, fmt.Errorf("invalid secret label %q: expected %s<SOURCE>.<PROPERTY>", labelName, SecretsPrefix)
		}

		source := result.Secrets[sourceID]
		switch {
		case property == "name":
			source.Name = strings.TrimSpace(value)
		case property == "kv":
			source.Mount = strings.TrimSpace(value)
		case property == "vault-path":
			source.Path = strings.TrimSpace(value)
		case strings.HasPrefix(property, "env."):
			environmentName := strings.TrimPrefix(property, "env.")
			if environmentName == "" {
				return Config{}, fmt.Errorf("secret source %q has an empty environment variable name", sourceID)
			}
			if strings.Contains(environmentName, "=") {
				return Config{}, fmt.Errorf("secret source %q environment name %q cannot contain '='", sourceID, environmentName)
			}
			fieldPath := strings.TrimSpace(value)
			if fieldPath == "" {
				return Config{}, fmt.Errorf("secret source %q: Vault field path for %q cannot be empty", sourceID, environmentName)
			}
			if source.Env == nil {
				source.Env = make(map[string]string)
			}
			source.Env[environmentName] = fieldPath
		default:
			return Config{}, fmt.Errorf("secret source %q has unknown property %q", sourceID, property)
		}
		result.Secrets[sourceID] = source
	}

	if len(result.Secrets) == 0 {
		return Config{}, fmt.Errorf("enabled service must define at least one label under %s", SecretsPrefix)
	}

	seenNames := make(map[string]string)
	seenExplicitEnvironment := make(map[string]string)
	for sourceID, source := range result.Secrets {
		if source.Name == "" {
			return Config{}, fmt.Errorf("secret source %q: name is required", sourceID)
		}
		if source.Mount == "" {
			return Config{}, fmt.Errorf("secret source %q: kv is required", sourceID)
		}
		if source.Path == "" {
			return Config{}, fmt.Errorf("secret source %q: vault-path is required", sourceID)
		}
		if previous, duplicate := seenNames[source.Name]; duplicate {
			return Config{}, fmt.Errorf("secret name %q is used by both source %q and %q", source.Name, previous, sourceID)
		}
		seenNames[source.Name] = sourceID

		// Auto-imported names are not available until Vault data is read. The
		// controller repeats this ownership check for the complete desired map.
		for environmentName := range source.Env {
			if previous, duplicate := seenExplicitEnvironment[environmentName]; duplicate {
				return Config{}, fmt.Errorf("environment variable %q is mapped by both source %q and %q", environmentName, previous, sourceID)
			}
			seenExplicitEnvironment[environmentName] = sourceID
		}
	}

	return result, nil
}

// ConfigHash detects changes to mount, path, source name or explicit mapping
// even when the old and new Vault documents happen to have the same version.
func ConfigHash(configuration Config) (string, error) {
	encoded, err := json.Marshal(configuration.Secrets)
	if err != nil {
		return "", fmt.Errorf("marshal secret source configuration: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func ParseAppliedVersions(serviceLabels map[string]string) (map[string]int, error) {
	result := make(map[string]int)
	raw := serviceLabels[AppliedVersionsLabel]
	if raw == "" {
		return result, nil
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parse controller label %s: %w", AppliedVersionsLabel, err)
	}
	return result, nil
}

func ParseManagedEnvironment(serviceLabels map[string]string) ([]string, error) {
	var result []string
	raw := serviceLabels[ManagedEnvLabel]
	if raw == "" {
		return result, nil
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parse controller label %s: %w", ManagedEnvLabel, err)
	}
	for _, name := range result {
		if name == "" || strings.Contains(name, "=") {
			return nil, fmt.Errorf("controller label %s contains an environment name that cannot be represented in Docker Env: %q", ManagedEnvLabel, name)
		}
	}
	sort.Strings(result)
	return result, nil
}

func MarshalState(versions map[string]int, managedEnvironment []string) (string, string, error) {
	versionsJSON, err := json.Marshal(versions)
	if err != nil {
		return "", "", fmt.Errorf("marshal applied versions: %w", err)
	}

	managedCopy := append([]string(nil), managedEnvironment...)
	sort.Strings(managedCopy)
	managedJSON, err := json.Marshal(managedCopy)
	if err != nil {
		return "", "", fmt.Errorf("marshal managed environment: %w", err)
	}

	return string(versionsJSON), string(managedJSON), nil
}

// HasControllerState is used for cleanup after enabled=false. It deliberately
// checks only controller-owned labels.
func HasControllerState(serviceLabels map[string]string) bool {
	return serviceLabels[AppliedVersionsLabel] != "" ||
		serviceLabels[ManagedEnvLabel] != "" ||
		serviceLabels[StateHashLabel] != "" ||
		serviceLabels[ConfigHashLabel] != ""
}
