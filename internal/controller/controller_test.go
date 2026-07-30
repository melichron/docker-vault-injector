package controller

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/swarm"

	"github.com/vyktory/docker-vault-injector/internal/labels"
	"github.com/vyktory/docker-vault-injector/internal/vaultclient"
)

type fakeDocker struct {
	updates []swarm.Service
}

func (d *fakeDocker) ListServices(context.Context) ([]swarm.Service, error) {
	return nil, nil
}

func (d *fakeDocker) InspectService(context.Context, string) (swarm.Service, error) {
	return swarm.Service{}, nil
}

func (d *fakeDocker) UpdateService(_ context.Context, service swarm.Service) error {
	d.updates = append(d.updates, service)
	return nil
}

func (d *fakeDocker) WatchServiceEvents(context.Context) (<-chan events.Message, <-chan error) {
	messages := make(chan events.Message)
	errorsChannel := make(chan error)
	close(messages)
	close(errorsChannel)
	return messages, errorsChannel
}

type fakeVault struct {
	versions map[string]int
	secrets  map[string]vaultclient.Secret
	reads    int
}

func (v *fakeVault) CurrentVersion(_ context.Context, mount, path string) (int, error) {
	return v.versions[mount+"/"+path], nil
}

func (v *fakeVault) ReadVersion(_ context.Context, mount, path string, _ int) (vaultclient.Secret, error) {
	v.reads++
	return v.secrets[mount+"/"+path], nil
}

func TestReconcileInjectsAndThenSkipsCurrentState(t *testing.T) {
	docker := &fakeDocker{}
	vault := &fakeVault{
		versions: map[string]int{"kv/apps/api": 7},
		secrets: map[string]vaultclient.Secret{
			"kv/apps/api": {
				Version: 7,
				Data: map[string]any{
					"username": "api-user",
					"database": map[string]any{"password": "correct horse"},
				},
			},
		},
	}
	c := testController(docker, vault)
	service := managedService()

	if err := c.ReconcileService(context.Background(), service); err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}
	if len(docker.updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(docker.updates))
	}
	updated := docker.updates[0]
	wantEnvironment := []string{
		"LOG_LEVEL=info",
		"DB_PASSWORD=correct horse",
		"DB_USER=api-user",
	}
	if got := updated.Spec.TaskTemplate.ContainerSpec.Env; !slices.Equal(got, wantEnvironment) {
		t.Fatalf("environment = %v, want %v", got, wantEnvironment)
	}
	if got := updated.Spec.Labels[labels.AppliedVersionsLabel]; got != `{"database":7}` {
		t.Fatalf("applied versions = %q", got)
	}

	if err := c.ReconcileService(context.Background(), updated); err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}
	if len(docker.updates) != 1 {
		t.Fatalf("second reconcile created an unnecessary update")
	}
	if vault.reads != 1 {
		t.Fatalf("Vault data reads = %d, want 1", vault.reads)
	}
}

func TestReconcileRepairsEnvironmentDriftWithoutVersionChange(t *testing.T) {
	docker := &fakeDocker{}
	vault := &fakeVault{
		versions: map[string]int{"kv/apps/api": 7},
		secrets: map[string]vaultclient.Secret{
			"kv/apps/api": {Version: 7, Data: map[string]any{
				"username": "api-user",
				"database": map[string]any{"password": "correct horse"},
			}},
		},
	}
	c := testController(docker, vault)

	if err := c.ReconcileService(context.Background(), managedService()); err != nil {
		t.Fatal(err)
	}
	drifted := docker.updates[0]
	drifted.Spec.TaskTemplate.ContainerSpec.Env[1] = "DB_PASSWORD=manually-changed"
	docker.updates = nil

	if err := c.ReconcileService(context.Background(), drifted); err != nil {
		t.Fatal(err)
	}
	if len(docker.updates) != 1 {
		t.Fatalf("drift should result in one update, got %d", len(docker.updates))
	}
}

func TestDisableRemovesOnlyManagedEnvironment(t *testing.T) {
	docker := &fakeDocker{}
	vault := &fakeVault{}
	c := testController(docker, vault)
	service := managedService()
	service.Spec.Labels[labels.EnabledLabel] = "false"
	service.Spec.Labels[labels.ManagedEnvLabel] = `["DB_PASSWORD","DB_USER"]`
	service.Spec.Labels[labels.AppliedVersionsLabel] = `{"database":7}`
	service.Spec.Labels[labels.StateHashLabel] = "hash"
	service.Spec.Labels[labels.ConfigHashLabel] = "config-hash"
	service.Spec.TaskTemplate.ContainerSpec.Env = []string{
		"LOG_LEVEL=info", "DB_PASSWORD=secret", "DB_USER=user",
	}

	if err := c.ReconcileService(context.Background(), service); err != nil {
		t.Fatal(err)
	}
	if len(docker.updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(docker.updates))
	}
	updated := docker.updates[0]
	if got := updated.Spec.TaskTemplate.ContainerSpec.Env; !slices.Equal(got, []string{"LOG_LEVEL=info"}) {
		t.Fatalf("environment after cleanup = %v", got)
	}
	for _, key := range []string{
		labels.ManagedEnvLabel,
		labels.AppliedVersionsLabel,
		labels.StateHashLabel,
		labels.ConfigHashLabel,
	} {
		if _, exists := updated.Spec.Labels[key]; exists {
			t.Fatalf("controller state label %s was not removed", key)
		}
	}
}

func TestReconcileAutomaticallyImportsTopLevelScalars(t *testing.T) {
	docker := &fakeDocker{}
	vault := &fakeVault{
		versions: map[string]int{"kv/apps/environment": 3},
		secrets: map[string]vaultclient.Secret{
			"kv/apps/environment": {
				Version: 3,
				Data: map[string]any{
					"FIRST_ENV":      "value",
					"SECOND_ENV":     "another_value",
					"PORT":           8080,
					"name-with-dash": "operator-choice",
				},
			},
		},
	}
	service := managedService()
	service.Spec.Labels = map[string]string{
		labels.EnabledLabel:                             "true",
		labels.SecretsPrefix + "application.name":       "application-vars",
		labels.SecretsPrefix + "application.kv":         "kv",
		labels.SecretsPrefix + "application.vault-path": "apps/environment",
	}

	if err := testController(docker, vault).ReconcileService(context.Background(), service); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"LOG_LEVEL=info",
		"FIRST_ENV=value",
		"PORT=8080",
		"SECOND_ENV=another_value",
		"name-with-dash=operator-choice",
	}
	if got := docker.updates[0].Spec.TaskTemplate.ContainerSpec.Env; !slices.Equal(got, want) {
		t.Fatalf("environment = %v, want %v", got, want)
	}
	if got := docker.updates[0].Spec.Labels[labels.ManagedEnvLabel]; got != `["FIRST_ENV","PORT","SECOND_ENV","name-with-dash"]` {
		t.Fatalf("managed environment label = %q", got)
	}
}

func TestReconcileRejectsEnvironmentCollisionBetweenAutomaticSources(t *testing.T) {
	docker := &fakeDocker{}
	vault := &fakeVault{
		versions: map[string]int{"kv/one": 1, "kv/two": 2},
		secrets: map[string]vaultclient.Secret{
			"kv/one": {Version: 1, Data: map[string]any{"TOKEN": "first"}},
			"kv/two": {Version: 2, Data: map[string]any{"TOKEN": "second"}},
		},
	}
	service := managedService()
	service.Spec.Labels = map[string]string{
		labels.EnabledLabel:                     "true",
		labels.SecretsPrefix + "one.name":       "one",
		labels.SecretsPrefix + "one.kv":         "kv",
		labels.SecretsPrefix + "one.vault-path": "one",
		labels.SecretsPrefix + "two.name":       "two",
		labels.SecretsPrefix + "two.kv":         "kv",
		labels.SecretsPrefix + "two.vault-path": "two",
	}

	err := testController(docker, vault).ReconcileService(context.Background(), service)
	if err == nil {
		t.Fatal("duplicate automatically imported environment variable should fail")
	}
	if len(docker.updates) != 0 {
		t.Fatal("a collision must not partially update the service")
	}
}

func TestReconcileAutomaticImportRejectsNestedValue(t *testing.T) {
	docker := &fakeDocker{}
	vault := &fakeVault{
		versions: map[string]int{"kv/nested": 1},
		secrets: map[string]vaultclient.Secret{
			"kv/nested": {Version: 1, Data: map[string]any{"DATABASE": map[string]any{"HOST": "db"}}},
		},
	}
	service := managedService()
	service.Spec.Labels = map[string]string{
		labels.EnabledLabel:                        "true",
		labels.SecretsPrefix + "nested.name":       "nested",
		labels.SecretsPrefix + "nested.kv":         "kv",
		labels.SecretsPrefix + "nested.vault-path": "nested",
	}

	err := testController(docker, vault).ReconcileService(context.Background(), service)
	if err == nil {
		t.Fatal("nested automatic value should fail instead of being JSON-encoded")
	}
	if len(docker.updates) != 0 {
		t.Fatal("an invalid value must not partially update the service")
	}
}

func TestResolveFieldRejectsComplexValues(t *testing.T) {
	_, err := resolveField(map[string]any{"nested": []any{"a"}}, "nested")
	if err == nil {
		t.Fatal("array value should not be accepted as an environment variable")
	}
}

func managedService() swarm.Service {
	return swarm.Service{
		ID:   "service-id",
		Meta: swarm.Meta{Version: swarm.Version{Index: 12}},
		Spec: swarm.ServiceSpec{
			Annotations: swarm.Annotations{
				Name: "example_api",
				Labels: map[string]string{
					labels.EnabledLabel:                               "true",
					labels.SecretsPrefix + "database.name":            "database",
					labels.SecretsPrefix + "database.kv":              "kv",
					labels.SecretsPrefix + "database.vault-path":      "apps/api",
					labels.SecretsPrefix + "database.env.DB_USER":     "username",
					labels.SecretsPrefix + "database.env.DB_PASSWORD": "database.password",
				},
			},
			TaskTemplate: swarm.TaskSpec{
				ContainerSpec: &swarm.ContainerSpec{Env: []string{"LOG_LEVEL=info"}},
			},
		},
	}
}

func testController(docker Docker, vault Vault) *Controller {
	return New(
		docker,
		vault,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		time.Minute,
		time.Minute,
		time.Second,
	)
}
