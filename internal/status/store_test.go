package status

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStorePreservesLastSuccessAcrossErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	first := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	second := first.Add(time.Minute)
	store.now = func() time.Time { return first }
	if err := store.Record(Service{
		ID:               "service-id",
		Name:             "stack_api",
		State:            StateReady,
		Gate:             GateOpen,
		Sources:          []string{"database"},
		EnvironmentNames: []string{"DB_PASSWORD"},
		Versions:         map[string]int{"database": 7},
	}, true); err != nil {
		t.Fatal(err)
	}

	store.now = func() time.Time { return second }
	if err := store.Record(Service{
		ID:      "service-id",
		Name:    "stack_api",
		State:   StateError,
		Gate:    GateOpen,
		Error:   "Vault unavailable",
		Sources: []string{"database"},
	}, false); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Services) != 1 {
		t.Fatalf("services = %d, want 1", len(snapshot.Services))
	}
	service := snapshot.Services[0]
	if service.LastSuccess == nil || !service.LastSuccess.Equal(first) {
		t.Fatalf("last success = %v, want %v", service.LastSuccess, first)
	}
	if !service.LastAttempt.Equal(second) {
		t.Fatalf("last attempt = %v, want %v", service.LastAttempt, second)
	}
	if service.Error != "Vault unavailable" {
		t.Fatalf("error = %q", service.Error)
	}
}

func TestWriteTableIsDeterministicAndSingleLine(t *testing.T) {
	lastSuccess := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	snapshot := Snapshot{Services: []Service{
		{
			Name:             "stack_api",
			State:            StateError,
			Gate:             GateClosed,
			Sources:          []string{"database", "common"},
			EnvironmentNames: []string{"DB_USER", "DB_PASSWORD"},
			Versions:         map[string]int{"database": 7, "common": 3},
			LastSuccess:      &lastSuccess,
			Error:            "Vault\n  unavailable",
		},
	}}

	var output bytes.Buffer
	if err := WriteTable(&output, snapshot); err != nil {
		t.Fatal(err)
	}
	rendered := output.String()
	for _, expected := range []string{
		"SERVICE",
		"stack_api",
		"common=3,database=7",
		"DB_USER,DB_PASSWORD",
		"Vault unavailable",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("table does not contain %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, "Vault\n") {
		t.Fatalf("error broke the table row:\n%s", rendered)
	}
}

func TestWriteYAMLIncludesCompleteStatus(t *testing.T) {
	lastAttempt := time.Date(2026, 8, 2, 10, 1, 0, 0, time.UTC)
	lastSuccess := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	snapshot := Snapshot{
		GeneratedAt: lastAttempt,
		Services: []Service{{
			ID:               "service-id",
			Name:             "stack_api",
			State:            StateError,
			Gate:             GateClosed,
			Sources:          []string{"database"},
			EnvironmentNames: []string{"DB_PASSWORD"},
			Versions:         map[string]int{"database": 7},
			LastAttempt:      lastAttempt,
			LastSuccess:      &lastSuccess,
			Error:            "Vault unavailable",
		}},
	}

	var output bytes.Buffer
	if err := WriteYAML(&output, snapshot); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"generated_at: 2026-08-02T10:01:00Z",
		"id: service-id",
		"name: stack_api",
		"state: error",
		"gate: closed",
		"- database",
		"- DB_PASSWORD",
		"database: 7",
		"last_attempt: 2026-08-02T10:01:00Z",
		"last_success: 2026-08-02T10:00:00Z",
		"error: Vault unavailable",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("YAML does not contain %q:\n%s", expected, output.String())
		}
	}
}

func TestPruneRemovesServicesMissingFromDockerList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range []Service{
		{ID: "one", Name: "one", State: StateReady, Gate: GateNotUsed},
		{ID: "two", Name: "two", State: StateReady, Gate: GateNotUsed},
	} {
		if err := store.Record(service, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Prune([]string{"two"}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Services) != 1 || snapshot.Services[0].ID != "two" {
		t.Fatalf("unexpected services after prune: %#v", snapshot.Services)
	}
}
