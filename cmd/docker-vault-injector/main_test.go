package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	statusview "github.com/melichron/docker-vault-injector/internal/status"
)

func TestRunStatusDoesNotRequireControllerConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.json")
	store, err := statusview.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Record(statusview.Service{
		ID:               "service-id",
		Name:             "stack_api",
		State:            statusview.StateReady,
		Gate:             statusview.GateOpen,
		EnvironmentNames: []string{"DB_PASSWORD"},
	}, true); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := runStatus(&output, path, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "stack_api") || !strings.Contains(output.String(), "DB_PASSWORD") {
		t.Fatalf("unexpected status output:\n%s", output.String())
	}

	output.Reset()
	if err := runStatus(&output, path, true); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"generated_at:",
		"id: service-id",
		"name: stack_api",
		"environment_names:",
		"- DB_PASSWORD",
		"last_attempt:",
		"last_success:",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("status YAML does not contain %q:\n%s", expected, output.String())
		}
	}
}
