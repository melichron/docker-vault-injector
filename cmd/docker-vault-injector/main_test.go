package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	statusview "github.com/melichron/docker-vault-injector/internal/status"
	"github.com/melichron/docker-vault-injector/internal/vaultclient"
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

func TestRunHealthChecksHeartbeatFreshness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.json")
	_, err := statusview.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := statusview.Read(path)
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := runHealth(&output, path, time.Minute, snapshot.HeartbeatAt.Add(59*time.Second)); err != nil {
		t.Fatalf("fresh heartbeat is unhealthy: %v", err)
	}
	if output.String() != "healthy\n" {
		t.Fatalf("health output = %q", output.String())
	}
	if err := runHealth(io.Discard, path, time.Minute, snapshot.HeartbeatAt.Add(61*time.Second)); err == nil {
		t.Fatal("stale heartbeat should fail")
	}
}

type retryingAuthenticator struct {
	attempts int
}

func (a *retryingAuthenticator) Authenticate(context.Context, vaultclient.AuthConfig) error {
	a.attempts++
	if a.attempts < 3 {
		return errors.New("Vault unavailable")
	}
	return nil
}

func TestInitialVaultAuthenticationRetriesUntilSuccess(t *testing.T) {
	authenticator := &retryingAuthenticator{}
	configuration := vaultclient.AuthConfig{
		AuthRetryInterval: time.Nanosecond,
		AuthRetryMaximum:  time.Nanosecond,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := authenticateVaultUntilSuccess(context.Background(), authenticator, configuration, logger, nil); err != nil {
		t.Fatal(err)
	}
	if authenticator.attempts != 3 {
		t.Fatalf("authentication attempts = %d, want 3", authenticator.attempts)
	}
}
