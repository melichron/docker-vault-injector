// Package environment contains the small but security-sensitive piece of code
// that edits ContainerSpec.Env. It never logs or otherwise exposes values.
package environment

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// AsMap applies Docker's usual "last entry wins" interpretation when duplicate
// names are present. The controller itself always emits a canonical list with
// no duplicates, but it must be defensive about input created by other tools.
func AsMap(entries []string) map[string]string {
	result := make(map[string]string, len(entries))
	for _, entry := range entries {
		name, value, found := strings.Cut(entry, "=")
		if !found {
			value = ""
		}
		result[name] = value
	}
	return result
}

// Select returns only the requested variables. Missing variables remain
// missing, which makes drift detection notice a manually removed value.
func Select(entries []string, names []string) map[string]string {
	all := AsMap(entries)
	result := make(map[string]string, len(names))
	for _, name := range names {
		if value, exists := all[name]; exists {
			result[name] = value
		}
	}
	return result
}

// Merge removes every previously or currently managed key, preserves all
// unrelated entries in their original order, and appends desired values in a
// deterministic order.
func Merge(entries []string, previouslyManaged []string, desired map[string]string) []string {
	managed := make(map[string]struct{}, len(previouslyManaged)+len(desired))
	for _, name := range previouslyManaged {
		managed[name] = struct{}{}
	}
	for name := range desired {
		managed[name] = struct{}{}
	}

	result := make([]string, 0, len(entries)+len(desired))
	for _, entry := range entries {
		name, _, _ := strings.Cut(entry, "=")
		if _, remove := managed[name]; !remove {
			result = append(result, entry)
		}
	}

	names := make([]string, 0, len(desired))
	for name := range desired {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result = append(result, name+"="+desired[name])
	}
	return result
}

func Remove(entries []string, names []string) []string {
	return Merge(entries, names, nil)
}

// Hash computes a deterministic digest of names and values. The digest lets
// the controller detect manual environment drift without reading every Vault
// secret on every polling cycle. Anyone allowed to inspect this label can also
// inspect ServiceSpec.Env, so the digest is not intended as a secrecy boundary.
func Hash(values map[string]string) string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	hasher := sha256.New()
	for _, name := range names {
		hasher.Write([]byte(name))
		hasher.Write([]byte{0})
		hasher.Write([]byte(values[name]))
		hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil))
}
