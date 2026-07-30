// Package labels owns the public label contract between a Swarm service and
// docker-vault-injector.
//
// Keeping all label names and JSON parsing here is intentional. The controller
// should reason about a typed Config, not about loosely structured strings.
package labels

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

const (
	Prefix = "io.github.docker-vault-injector"

	EnabledLabel         = Prefix + ".enabled"
	SecretsLabel         = Prefix + ".secrets"
	AppliedVersionsLabel = Prefix + ".applied-versions"
	ManagedEnvLabel      = Prefix + ".managed-env"
	StateHashLabel       = Prefix + ".state-hash"
)

var environmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Config is the user-controlled portion of the service configuration.
type Config struct {
	Enabled bool
	Secrets map[string]SecretSource
}

// SecretSource describes one Vault KV v2 document and how fields from that
// document map to process environment variables.
type SecretSource struct {
	Mount string            `json:"mount"`
	Path  string            `json:"path"`
	Env   map[string]string `json:"env"`
}

// ParseConfig reads user-controlled labels. Unknown labels are ignored so new
// controller versions can add state without breaking older versions.
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

	rawSecrets := serviceLabels[SecretsLabel]
	if rawSecrets == "" {
		return Config{}, fmt.Errorf("%s is required when injection is enabled", SecretsLabel)
	}
	if err := json.Unmarshal([]byte(rawSecrets), &result.Secrets); err != nil {
		return Config{}, fmt.Errorf("parse %s as JSON: %w", SecretsLabel, err)
	}
	if len(result.Secrets) == 0 {
		return Config{}, fmt.Errorf("%s must contain at least one secret", SecretsLabel)
	}

	seenEnvironment := make(map[string]string)
	for name, secret := range result.Secrets {
		if name == "" {
			return Config{}, fmt.Errorf("secret source name cannot be empty")
		}
		if secret.Mount == "" {
			return Config{}, fmt.Errorf("secret %q: mount is required", name)
		}
		if secret.Path == "" {
			return Config{}, fmt.Errorf("secret %q: path is required", name)
		}
		if len(secret.Env) == 0 {
			return Config{}, fmt.Errorf("secret %q: env mapping cannot be empty", name)
		}
		for environment, fieldPath := range secret.Env {
			if !environmentName.MatchString(environment) {
				return Config{}, fmt.Errorf("secret %q: %q is not a valid environment variable name", name, environment)
			}
			if fieldPath == "" {
				return Config{}, fmt.Errorf("secret %q: Vault field path for %s cannot be empty", name, environment)
			}
			if previous, duplicate := seenEnvironment[environment]; duplicate {
				return Config{}, fmt.Errorf("environment variable %s is mapped by both %q and %q", environment, previous, name)
			}
			seenEnvironment[environment] = name
		}
	}

	return result, nil
}

// DesiredEnvironmentNames returns a stable, sorted list. Stable ordering keeps
// ServiceSpec updates deterministic and makes tests and docker service inspect
// output easier to read.
func (c Config) DesiredEnvironmentNames() []string {
	var result []string
	for _, secret := range c.Secrets {
		for name := range secret.Env {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
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
		if !environmentName.MatchString(name) {
			return nil, fmt.Errorf("controller label %s contains invalid environment name %q", ManagedEnvLabel, name)
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
		serviceLabels[StateHashLabel] != ""
}
