// Package status stores a small, secret-free snapshot of the controller's
// latest reconciliation results. The long-running process writes the file;
// the status subcommands, executed in the same container, read it.
package status

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	StateReady    = "ready"
	StateDisabled = "disabled"
	StateError    = "error"

	GateNotUsed = "-"
	GateClosed  = "closed"
	GateOpen    = "open"
	GateMissing = "missing"
)

// Service is the public, on-disk status representation. It intentionally
// contains environment names but never environment values, Vault paths,
// credentials, or secret data.
type Service struct {
	ID               string         `json:"id" yaml:"id"`
	Name             string         `json:"name" yaml:"name"`
	State            string         `json:"state" yaml:"state"`
	Gate             string         `json:"gate" yaml:"gate"`
	Sources          []string       `json:"sources,omitempty" yaml:"sources"`
	EnvironmentNames []string       `json:"environment_names,omitempty" yaml:"environment_names"`
	Versions         map[string]int `json:"versions,omitempty" yaml:"versions"`
	LastAttempt      time.Time      `json:"last_attempt" yaml:"last_attempt"`
	LastSuccess      *time.Time     `json:"last_success,omitempty" yaml:"last_success"`
	Error            string         `json:"error,omitempty" yaml:"error"`
	Warnings         []string       `json:"warnings,omitempty" yaml:"warnings"`
	UpdateAttempted  bool           `json:"-" yaml:"-"`
}

type Snapshot struct {
	GeneratedAt time.Time `json:"generated_at" yaml:"generated_at"`
	HeartbeatAt time.Time `json:"heartbeat_at" yaml:"heartbeat_at"`
	Services    []Service `json:"services" yaml:"services"`
}

// Store serializes every mutation and writes a complete snapshot through an
// atomic rename. A concurrently executed status command therefore observes
// either the previous complete file or the next complete file, never partial
// JSON.
type Store struct {
	mu        sync.Mutex
	path      string
	services  map[string]Service
	heartbeat time.Time
	now       func() time.Time
}

func NewStore(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("status file path cannot be empty")
	}
	store := &Store{
		path:     path,
		services: make(map[string]Service),
		now:      time.Now,
	}
	store.heartbeat = store.now().UTC()
	if err := store.writeLocked(); err != nil {
		return nil, err
	}
	return store, nil
}

// Record replaces the current view of one service. On a failed reconciliation
// the last successful timestamp is retained, which makes an outage visible
// without losing when the service last converged.
func (s *Store) Record(service Service, successful bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now().UTC()
	service.LastAttempt = now
	if successful {
		service.LastSuccess = &now
	} else if previous, exists := s.services[service.ID]; exists {
		service.LastSuccess = previous.LastSuccess
	}
	service.Sources = sortedCopy(service.Sources)
	service.EnvironmentNames = sortedCopy(service.EnvironmentNames)
	service.Versions = cloneVersions(service.Versions)
	if previous, exists := s.services[service.ID]; exists && !service.UpdateAttempted {
		service.Warnings = append([]string(nil), previous.Warnings...)
	} else {
		service.Warnings = append([]string(nil), service.Warnings...)
	}
	s.services[service.ID] = service
	s.heartbeat = now
	return s.writeLocked()
}

// Heartbeat proves that the controller's main reconciliation loop is still
// making progress even when there are no injector-managed services. It is
// intentionally independent from whether Docker or Vault operations succeed.
func (s *Store) Heartbeat() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.heartbeat = s.now().UTC()
	return s.writeLocked()
}

func (s *Store) Remove(serviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.services[serviceID]; !exists {
		return nil
	}
	delete(s.services, serviceID)
	return s.writeLocked()
}

// Prune removes services that disappeared from the latest successful Docker
// ServiceList. It does not run when listing Docker itself fails.
func (s *Store) Prune(activeServiceIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	active := make(map[string]struct{}, len(activeServiceIDs))
	for _, serviceID := range activeServiceIDs {
		active[serviceID] = struct{}{}
	}
	changed := false
	for serviceID := range s.services {
		if _, exists := active[serviceID]; !exists {
			delete(s.services, serviceID)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.writeLocked()
}

func (s *Store) writeLocked() error {
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create status directory %s: %w", directory, err)
	}

	temporary, err := os.CreateTemp(directory, ".docker-vault-injector-status-*")
	if err != nil {
		return fmt.Errorf("create temporary status file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("set status file permissions: %w", err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(s.snapshotLocked()); err != nil {
		temporary.Close()
		return fmt.Errorf("encode status snapshot: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close status snapshot: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace status snapshot %s: %w", s.path, err)
	}
	return nil
}

func (s *Store) snapshotLocked() Snapshot {
	services := make([]Service, 0, len(s.services))
	for _, service := range s.services {
		services = append(services, service)
	}
	sort.Slice(services, func(i, j int) bool {
		if services[i].Name == services[j].Name {
			return services[i].ID < services[j].ID
		}
		return services[i].Name < services[j].Name
	})
	return Snapshot{
		GeneratedAt: s.now().UTC(),
		HeartbeatAt: s.heartbeat,
		Services:    services,
	}
}

func Read(path string) (Snapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("open status snapshot %s: %w", path, err)
	}
	defer file.Close()

	var snapshot Snapshot
	if err := json.NewDecoder(file).Decode(&snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("decode status snapshot %s: %w", path, err)
	}
	return snapshot, nil
}

// CheckHealth validates only liveness of the controller loop. Service errors
// and temporary Vault outages do not make the process unhealthy and therefore
// cannot trigger a restart storm.
func CheckHealth(snapshot Snapshot, maximumAge time.Duration, now time.Time) error {
	if maximumAge <= 0 {
		return fmt.Errorf("health maximum age must be greater than zero")
	}
	if snapshot.HeartbeatAt.IsZero() {
		return fmt.Errorf("controller heartbeat has not been recorded")
	}
	age := now.Sub(snapshot.HeartbeatAt)
	if age > maximumAge {
		return fmt.Errorf("controller heartbeat is stale: age %s exceeds %s", age.Round(time.Second), maximumAge)
	}
	return nil
}

func WriteTable(writer io.Writer, snapshot Snapshot) error {
	if len(snapshot.Services) == 0 {
		_, err := fmt.Fprintln(writer, "No injector-managed services have been observed yet.")
		return err
	}

	table := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "SERVICE\tSTATE\tGATE\tSOURCES\tENVIRONMENT\tVAULT VERSIONS\tLAST SUCCESS\tERROR\tWARNINGS"); err != nil {
		return err
	}
	for _, service := range snapshot.Services {
		lastSuccess := "-"
		if service.LastSuccess != nil {
			lastSuccess = service.LastSuccess.Local().Format(time.RFC3339)
		}
		if _, err := fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			service.Name,
			service.State,
			service.Gate,
			valueOrDash(strings.Join(service.Sources, ",")),
			valueOrDash(strings.Join(service.EnvironmentNames, ",")),
			formatVersions(service.Versions),
			lastSuccess,
			valueOrDash(singleLine(service.Error)),
			valueOrDash(singleLine(strings.Join(service.Warnings, "; "))),
		); err != nil {
			return err
		}
	}
	return table.Flush()
}

// WriteYAML renders the complete secret-free snapshot. Unlike the compact
// table, it includes service IDs, attempt timestamps, empty fields, and the
// snapshot generation time so scripts can consume the output without reading
// the controller's internal JSON file directly.
func WriteYAML(writer io.Writer, snapshot Snapshot) error {
	encoder := yaml.NewEncoder(writer)
	encoder.SetIndent(2)
	defer encoder.Close()
	if err := encoder.Encode(snapshot); err != nil {
		return fmt.Errorf("encode status YAML: %w", err)
	}
	return nil
}

func formatVersions(versions map[string]int) string {
	if len(versions) == 0 {
		return "-"
	}
	names := make([]string, 0, len(versions))
	for name := range versions {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, fmt.Sprintf("%s=%d", name, versions[name]))
	}
	return strings.Join(result, ",")
}

func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func valueOrDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func sortedCopy(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func cloneVersions(versions map[string]int) map[string]int {
	if versions == nil {
		return nil
	}
	result := make(map[string]int, len(versions))
	for name, version := range versions {
		result[name] = version
	}
	return result
}
