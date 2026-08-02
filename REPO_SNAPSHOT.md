# Repository Snapshot

Generated: 2026-08-02 UTC

## Included files

```text
./.dockerignore
./.github/workflows/publish-image.yml
./.gitignore
./AGENTS.md
./cmd/docker-vault-injector/main_test.go
./cmd/docker-vault-injector/main.go
./Dockerfile
./examples/postgres-stack.yaml
./examples/setup-approle.md
./examples/stack.yaml
./examples/vault-policy.hcl
./go.mod
./go.sum
./internal/config/config_test.go
./internal/config/config.go
./internal/controller/controller_test.go
./internal/controller/controller.go
./internal/dockerclient/client.go
./internal/environment/environment_test.go
./internal/environment/environment.go
./internal/labels/labels_test.go
./internal/labels/labels.go
./internal/retry/backoff_test.go
./internal/retry/backoff.go
./internal/status/store_test.go
./internal/status/store.go
./internal/vaultclient/client_test.go
./internal/vaultclient/client.go
./Makefile
./README.md
```

## File contents

### `.dockerignore`

```text
.git
.gitignore
bin
coverage.out
Dockerfile*
README.md
AGENTS.md
tmp/*
examples/*
gen-repo-snapshot.sh
REPO_SNAPSHOT.md
Makefile
```

### `.github/workflows/publish-image.yml`

```yaml
name: Build and publish container image

on:
  push:
    tags:
      - "v*"

env:
  IMAGE_NAME: ghcr.io/melichron/docker-vault-injector

jobs:
  publish:
    name: Publish multi-platform image
    runs-on: ubuntu-latest

    permissions:
      contents: read
      packages: write

    steps:
      - name: Checkout repository
        uses: actions/checkout@v6

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v4

      - name: Log in to GitHub Container Registry
        uses: docker/login-action@v4
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Generate image metadata
        id: meta
        uses: docker/metadata-action@v6
        with:
          images: ${{ env.IMAGE_NAME }}
          tags: |
            type=ref,event=tag
            type=raw,value=latest

      - name: Build and push image
        uses: docker/build-push-action@v7
        with:
          context: .
          file: ./Dockerfile
          push: true
          platforms: |
            linux/amd64
            linux/arm64
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

### `.gitignore`

```text
/bin/
/coverage.out
*.test
.DS_Store
tmp/*
```

### `AGENTS.md`

````markdown
# AGENTS.md

This file is durable project context for humans and coding agents. Read it before changing the repository.

## Project goal

`docker-vault-injector` is a small reconciliation controller for Docker Swarm. It observes **service-level** `deploy.labels`, reads HashiCorp Vault KV v2 values, and writes them to `ServiceSpec.TaskTemplate.ContainerSpec.Env`. Flat documents are imported as-is by default; explicit field mappings are optional. Docker Swarm performs task replacement and rollout.

The project intentionally does not support standalone Docker containers. A single host should run a single-node Swarm instead.

## Product boundaries

- Docker Swarm Services only.
- HashiCorp Vault KV v2 only for the first release.
- Environment-variable injection is intentional. Do not replace it with Docker Secrets unless the product scope explicitly changes.
- Secret visibility through `docker service inspect` is an accepted property of the threat model.
- Do not build images, manage stack names, or become a general deployment tool.
- Do not add a large controller framework. The explicit loop in `internal/controller` is preferred.

## Non-negotiable invariants

1. Never log, metric-label, or error-format a secret value.
2. Never blank or remove currently working values because Vault is unavailable or a new configuration is invalid.
3. Read Vault metadata first, then read the **exact** selected KV version. Do not fetch "latest" after metadata because that creates a version race.
4. Pass the inspected Docker `Version.Index` to `ServiceUpdate`. A conflict must be retried by a fresh inspect/reconcile, not by overwriting newer state.
5. Preserve every ServiceSpec field and environment variable not owned by this controller.
6. Keep controller-generated environment order deterministic.
7. Docker events are only low-latency triggers. Periodic full reconciliation remains the correctness mechanism.
8. A service update produced by this controller will produce another Docker event. Reconciliation must remain idempotent and terminate without another update.
9. Service configuration belongs in `deploy.labels`, not task/container labels.
10. Only one source may own a given environment variable.
11. AppRole auth mount path is mandatory and must be a full `auth/<mount>` path without `/login`.
12. Never log RoleID, SecretID, client token, or renewal response contents.
13. AppRole credential files must be re-read on every login so operators can rotate credentials.
14. Token lifecycle must retain proactive `lookup-self`, renewal before TTL, and re-login after revoke/expiry/renewal failure.
15. Do not enforce shell-style environment naming conventions. Reject only names that Docker's `NAME=value` representation cannot encode unambiguously: empty names and names containing `=`.
16. A collision between automatically imported and/or explicitly mapped environment names must abort reconciliation before `ServiceUpdate`.
17. When bootstrap gating is enabled, remove the reserved placement constraint only in the same `ServiceUpdate` that writes a fully resolved environment and controller state.
18. Any Vault, parsing, mapping, collision, or Docker error must leave the bootstrap gate closed. Never alter operator-owned placement constraints.
19. Status snapshots may contain service/source/environment names and safe error text, but never secret values, Vault data, RoleID, SecretID, or client tokens.
20. Failure to write observability state must never fail or delay secret reconciliation.
21. Health reports reconciliation-loop liveness only. Vault or per-service failures must remain visible without making the controller unhealthy and causing restart storms.
22. Docker warnings from successful ServiceUpdate calls may be logged and stored as warning text, but must never change a successful update into a failed reconciliation.

## Public label contract

User-controlled:

- `io.github.docker-vault-injector.enabled`
- `io.github.docker-vault-injector.bootstrap-gate` (optional boolean)
- `io.github.docker-vault-injector.secrets.<source>.name`
- `io.github.docker-vault-injector.secrets.<source>.kv`
- `io.github.docker-vault-injector.secrets.<source>.vault-path`
- `io.github.docker-vault-injector.secrets.<source>.env.<environment-name>` (optional)

Controller-controlled:

- `io.github.docker-vault-injector.applied-versions`
- `io.github.docker-vault-injector.managed-env`
- `io.github.docker-vault-injector.state-hash`
- `io.github.docker-vault-injector.config-hash`

All constants and parsing live in `internal/labels`. Do not duplicate literal label strings elsewhere.

Labels are grouped by the arbitrary `<source>` segment. `name`, `kv`, and `vault-path` are required. `name` must be unique across sources; `kv` is the KV v2 mount; `vault-path` is its logical path.

With no `.env.*` labels for a source, import every top-level scalar Vault field using the field name unchanged. If one or more `.env.*` labels exist, import only those mappings; the suffix is the target environment name and the label value is a dotted Vault field path. Objects, arrays, and null are never encoded into environment values.

Bootstrap-gated services must include this exact placement constraint in the stack specification:

```text
node.labels.io.github.docker-vault-injector.gate==open
```

No node may carry the matching `io.github.docker-vault-injector.gate=open` label. The controller owns only this exact reserved constraint. It preserves every other constraint, removes the gate atomically with successful injection, and never removes it while injection is disabled or failing. The gate and `bootstrap-gate=true` should remain in the stack file so every subsequent `docker stack deploy` is gated as well.

The old multiline `io.github.docker-vault-injector.secrets` JSON label is not supported. The flat label schema is the supported public contract. Any future schema change is a public API change and needs an explicit migration story.

## Reconciliation algorithm

For an enabled service:

1. Parse and validate labels.
2. Parse controller-owned applied versions, managed names, state hash, and config hash.
3. Detect the reserved bootstrap constraint. Reject it without explicit opt-in, and reject a new or changed gated configuration that omitted it.
4. Read `current_version` metadata for every configured source.
5. Compare versions, configuration hash, the hash of current previously-managed environment values, and gate state.
6. If all match and no gate remains, return without reading Vault data or updating Docker.
7. Otherwise read the exact current version of every source.
8. Auto-import all top-level scalar fields for sources without mappings; otherwise resolve the explicit dotted field paths.
9. Reject null/object/array values and reject duplicate environment ownership across every source.
10. Remove previously managed keys, preserve unrelated environment entries, append desired keys in sorted order.
11. Update controller state labels and, when enabled, remove only the reserved gate constraint.
12. Call one `ServiceUpdate` with the version from the inspected service. Preserve and report any warnings returned with a successful update.

For a disabled service with controller state, remove previously managed environment keys and controller state labels. Keep the user's `enabled=false` label and never remove a bootstrap constraint while disabled.

After every tracked-service attempt, record its result in the local status store. Preserve the previous `last_success` timestamp when a later attempt fails. After each successful full Docker service listing, prune records for deleted services. Services without injector configuration or controller state must not remain in the table.

## Code map

- `cmd/docker-vault-injector/main.go`: dependency wiring, logging, signal handling.
- `internal/config`: process-level environment settings only.
- `internal/controller`: orchestration and reconciliation policy.
- `internal/dockerclient`: thin adapter over `github.com/moby/moby/client`.
- `internal/vaultclient`: thin adapter over `github.com/hashicorp/vault/api`.
- `internal/labels`: public schema, validation, state serialization.
- `internal/environment`: deterministic Env merge/removal/hash.
- `internal/retry`: small exponential backoff with jitter shared by Docker and Vault retry loops.
- `internal/status`: atomic secret-free snapshot storage plus terminal table and complete YAML rendering.
- `examples`: operator-facing examples.

Prefer straightforward functions over generalized repositories, factories, event buses, or plugin systems. Interfaces belong at Docker and Vault boundaries so unit tests can use fakes.

## Development commands

```bash
make fmt
make test
make vet
make build
make docker-build
```

Before handing off a change, run at least `go test ./...` and `go vet ./...`. Add focused tests for label parsing, environment ownership, version transitions, drift, and failure behavior.

## Dependencies

Use the current supported split Moby modules:

```text
github.com/moby/moby/client
github.com/moby/moby/api
```

Do not reintroduce the deprecated monolithic `github.com/docker/docker` import path. Use HashiCorp's official `github.com/hashicorp/vault/api` client.

## Known limitations and likely next steps

- AppRole uses reusable RoleID/SecretID credentials; response-wrapped SecretID delivery is not implemented yet.
- One controller replica is expected. Multiple replicas are mostly protected by Docker optimistic locking but can cause noisy conflicts; implement leader election before recommending HA replicas.
- Reapplying a stack file can cause one rollout from `docker stack deploy` and a second rollout from reinjection.
- Bootstrap gating prevents uninjected task revisions only when the incoming stack/service specification contains the reserved constraint. The label alone cannot intercept the Swarm scheduler.
- `docker-vault-injector status` and `status-yaml` read a task-local snapshot. They do not aggregate multiple controller replicas and are temporarily empty after an injector task restart until reconciliation runs.
- There is no Prometheus metrics endpoint. Health is intentionally exposed as a local heartbeat-based command rather than an HTTP endpoint.
- Full end-to-end tests against a real single-node Swarm and Vault dev server are not present.

When implementing these, preserve the small explicit architecture and update both README.md and this file.
````

### `cmd/docker-vault-injector/main_test.go`

```go
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
```

### `cmd/docker-vault-injector/main.go`

```go
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/melichron/docker-vault-injector/internal/config"
	"github.com/melichron/docker-vault-injector/internal/controller"
	"github.com/melichron/docker-vault-injector/internal/dockerclient"
	"github.com/melichron/docker-vault-injector/internal/retry"
	statusview "github.com/melichron/docker-vault-injector/internal/status"
	"github.com/melichron/docker-vault-injector/internal/vaultclient"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "docker-vault-injector:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 {
		if len(os.Args) == 2 {
			switch os.Args[1] {
			case "status":
				return runStatus(os.Stdout, config.StatusFileFromEnvironment(), false)
			case "status-yaml":
				return runStatus(os.Stdout, config.StatusFileFromEnvironment(), true)
			case "health":
				maximumAge, err := config.HealthMaxAgeFromEnvironment()
				if err != nil {
					return err
				}
				return runHealth(os.Stdout, config.StatusFileFromEnvironment(), maximumAge, time.Now())
			}
		}
		return fmt.Errorf("usage: docker-vault-injector [status|status-yaml|health]")
	}

	configuration, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: configuration.LogLevel,
	}))
	slog.SetDefault(logger)
	statusStore, err := statusview.NewStore(configuration.StatusFile)
	if err != nil {
		// Status reporting is observability only. A read-only filesystem or
		// another status-file problem must not stop secret reconciliation.
		logger.Warn("status snapshot is disabled", "path", configuration.StatusFile, "error", err)
		statusStore = nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	docker, err := dockerclient.NewFromEnvironment()
	if err != nil {
		return err
	}
	defer docker.Close()

	vault, err := vaultclient.NewFromEnvironment()
	if err != nil {
		return err
	}
	authConfiguration := vaultclient.AuthConfig{
		Method:              configuration.VaultAuth.Method,
		AppRoleAuthPath:     configuration.VaultAuth.AppRoleAuthPath,
		AppRoleRoleID:       configuration.VaultAuth.AppRoleRoleID,
		AppRoleRoleIDFile:   configuration.VaultAuth.AppRoleRoleIDFile,
		AppRoleSecretID:     configuration.VaultAuth.AppRoleSecretID,
		AppRoleSecretIDFile: configuration.VaultAuth.AppRoleSecretIDFile,
		Token:               configuration.VaultAuth.Token,
		TokenFile:           configuration.VaultAuth.TokenFile,
		TokenCheckInterval:  configuration.VaultAuth.TokenCheckInterval,
		AuthRetryInterval:   configuration.VaultAuth.AuthRetryInterval,
		AuthRetryMaximum:    configuration.VaultAuth.AuthRetryMaximum,
	}
	if err := authenticateVaultUntilSuccess(ctx, vault, authConfiguration, logger, statusStore); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	if vault.MaintainsToken() {
		go vault.RunTokenLifecycle(ctx, logger)
	}

	reconciler := controller.New(
		docker,
		vault,
		logger,
		configuration.PollInterval,
		configuration.ReconcileTimeout,
		configuration.EventRetryInterval,
		configuration.EventRetryMaximum,
		statusStore,
	)

	logger.Info("starting docker-vault-injector",
		"poll_interval", configuration.PollInterval,
		"reconcile_timeout", configuration.ReconcileTimeout,
		"vault_auth_method", configuration.VaultAuth.Method,
		"status_file", configuration.StatusFile,
		"health_max_age", configuration.HealthMaxAge,
	)
	if err := reconciler.Run(ctx); err != nil {
		return fmt.Errorf("run controller: %w", err)
	}
	logger.Info("docker-vault-injector stopped")
	return nil
}

type vaultAuthenticator interface {
	Authenticate(context.Context, vaultclient.AuthConfig) error
}

func authenticateVaultUntilSuccess(
	ctx context.Context,
	vault vaultAuthenticator,
	configuration vaultclient.AuthConfig,
	logger *slog.Logger,
	statusStore *statusview.Store,
) error {
	backoff := retry.NewBackoff(configuration.AuthRetryInterval, configuration.AuthRetryMaximum)
	for ctx.Err() == nil {
		touchStatus(statusStore, logger)
		if err := vault.Authenticate(ctx, configuration); err == nil {
			return nil
		} else {
			delay := backoff.Next()
			// A Vault request may consume most of the health window. Record
			// progress again before entering the retry delay.
			touchStatus(statusStore, logger)
			logger.Error("initial Vault authentication failed; will retry",
				"retry_after", delay,
				"error", err,
			)
			if !waitForContext(ctx, delay) {
				break
			}
		}
	}
	return ctx.Err()
}

func touchStatus(store *statusview.Store, logger *slog.Logger) {
	if store == nil {
		return
	}
	if err := store.Heartbeat(); err != nil {
		logger.Warn("cannot write controller heartbeat", "error", err)
	}
}

func waitForContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func runStatus(writer io.Writer, path string, yamlOutput bool) error {
	snapshot, err := statusview.Read(path)
	if err != nil {
		return err
	}
	if yamlOutput {
		return statusview.WriteYAML(writer, snapshot)
	}
	return statusview.WriteTable(writer, snapshot)
}

func runHealth(writer io.Writer, path string, maximumAge time.Duration, now time.Time) error {
	snapshot, err := statusview.Read(path)
	if err != nil {
		return err
	}
	if err := statusview.CheckHealth(snapshot, maximumAge, now); err != nil {
		return err
	}
	_, err = fmt.Fprintln(writer, "healthy")
	return err
}
```

### `Dockerfile`

```dockerfile
# syntax=docker/dockerfile:1

# The controller is a single static Go binary. A multi-stage build keeps the
# runtime image small and avoids shipping a compiler or shell next to a process
# that has access to the Docker manager socket.
FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" \
    -o /out/docker-vault-injector ./cmd/docker-vault-injector

FROM scratch
LABEL org.opencontainers.image.source="https://github.com/melichron/docker-vault-injector"
LABEL org.opencontainers.image.description="Inject HashiCorp Vault KV secrets into Docker Swarm service environments"
ENV PATH=/bin

# Vault should use HTTPS in production. The CA bundle is needed for public CAs;
# private CAs can additionally be mounted and selected with VAULT_CACERT.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/docker-vault-injector /bin/docker-vault-injector

ENTRYPOINT ["/bin/docker-vault-injector"]

```

### `examples/postgres-stack.yaml`

```yaml
version: "3.8"

# This stack demonstrates the bootstrap gate with an initialization-sensitive
# image. The Postgres task cannot be scheduled until the injector has read
# Vault, written POSTGRES_* to ContainerSpec.Env, and removed only its reserved
# placement constraint in the same ServiceUpdate.

services:
  injector:
    image: ghcr.io/melichron/docker-vault-injector:latest
    environment:
      VAULT_ADDR: "https://vault.example.com:8200"
      VAULT_AUTH_METHOD: "approle"
      VAULT_APPROLE_AUTH_PATH: "auth/docker-swarm"
      VAULT_APPROLE_ROLE_ID_FILE: "/run/secrets/vault_approle_role_id"
      VAULT_APPROLE_SECRET_ID_FILE: "/run/secrets/vault_approle_secret_id"
      VAULT_TOKEN_CHECK_INTERVAL: "30s"
      INJECTOR_POLL_INTERVAL: "30s"
      INJECTOR_LOG_LEVEL: "info"
    secrets:
      - vault_approle_role_id
      - vault_approle_secret_id
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    healthcheck:
      test: ["CMD", "docker-vault-injector", "health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    deploy:
      replicas: 1
      placement:
        constraints:
          - node.role==manager
      restart_policy:
        condition: any
        delay: 5s

  postgres:
    image: postgres:17-alpine
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U \"$${POSTGRES_USER}\" -d \"$${POSTGRES_DB}\""]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 20s
    stop_grace_period: 60s
    deploy:
      replicas: 1
      placement:
        constraints:
          # This is a real placement constraint and remains after injection.
          # The example targets a single-node Swarm. Pin stateful workloads to
          # durable storage explicitly in a multi-node cluster.
          - node.role==manager

          # Never add this label to a node. The injector removes this exact
          # constraint only after every Vault value has been resolved.
          - node.labels.io.github.docker-vault-injector.gate==open
      labels:
        io.github.docker-vault-injector.enabled: "true"
        io.github.docker-vault-injector.bootstrap-gate: "true"

        # Store POSTGRES_USER, POSTGRES_PASSWORD, and POSTGRES_DB as top-level
        # scalar keys in this Vault KV v2 document. They are imported as-is.
        io.github.docker-vault-injector.secrets.postgres.name: "postgres-init"
        io.github.docker-vault-injector.secrets.postgres.kv: "kv"
        io.github.docker-vault-injector.secrets.postgres.vault-path: "apps/example/postgres"
      update_config:
        order: stop-first
        parallelism: 1
        failure_action: rollback
      restart_policy:
        condition: any
        delay: 5s

volumes:
  postgres_data:

secrets:
  vault_approle_role_id:
    external: true
  vault_approle_secret_id:
    external: true
```

### `examples/setup-approle.md`

````markdown
# Создание Vault policy и AppRole

Этот пример создаёт отдельный AppRole для `docker-vault-injector`, смонтированный в нестандартном path `auth/docker-swarm`. Injector должен получить именно полный mount path `auth/docker-swarm`, но без `/login`.

Команды выполняются Vault-оператором с правами на создание auth methods, policies и AppRole. Значения можно изменить под свою инфраструктуру:

```bash
export VAULT_ADDR=https://vault.example.com:8200
export AUTH_PATH=docker-swarm
export ROLE_NAME=docker-vault-injector
export POLICY_NAME=docker-vault-injector
```

В `AUTH_PATH` для Vault CLI указывается имя mount без префикса `auth/`. В конфигурации injector тот же mount будет записан как `VAULT_APPROLE_AUTH_PATH=auth/docker-swarm`.

## 1. Включить AppRole auth method

```bash
vault auth enable -path="$AUTH_PATH" approle
```

Если auth method уже существует, этот шаг выполнять повторно не нужно. Проверить mounts можно командой:

```bash
vault auth list
```

## 2. Создать policy

Готовый файл находится рядом: [vault-policy.hcl](vault-policy.hcl).

```bash
vault policy write "$POLICY_NAME" examples/vault-policy.hcl
```

Policy разрешает:

- читать metadata и data под `kv/apps/*`;
- проверять текущий token через `auth/token/lookup-self`;
- продлевать token через `auth/token/renew-self`.

Если KV v2 смонтирован не в `kv` или секреты лежат вне `apps/*`, измените оба KV path в policy.

## 3. Создать AppRole

```bash
vault write "auth/$AUTH_PATH/role/$ROLE_NAME" \
  token_type=service \
  token_policies="$POLICY_NAME" \
  token_period=20m \
  token_explicit_max_ttl=0 \
  token_num_uses=0 \
  secret_id_num_uses=0 \
  secret_id_ttl=0
```

Здесь намеренно используется periodic service token:

- `token_period=20m` выдаёт renewable token с TTL 20 минут;
- injector проверяет его состояние и продлевает примерно на 2/3 TTL;
- если token revoked или renewal не удался, injector снова выполняет AppRole login;
- `secret_id_num_uses=0` позволяет повторный login с тем же SecretID;
- `secret_id_ttl=0` не даёт SecretID истечь самостоятельно.

Бессрочный и многократно используемый SecretID удобен для начальной эксплуатации, но является долгоживущим credential. Его следует хранить как Docker Secret, ограничить policy минимально необходимыми paths и периодически ротировать. Более строгая схема может использовать короткоживущий wrapped SecretID, но тогда нужен отдельный механизм его доставки и обновления.

## 4. Получить RoleID и SecretID

```bash
vault read -field=role_id \
  "auth/$AUTH_PATH/role/$ROLE_NAME/role-id" \
  > role-id.txt

vault write -field=secret_id -f \
  "auth/$AUTH_PATH/role/$ROLE_NAME/secret-id" \
  > secret-id.txt
```

Не выводите SecretID в CI logs. Локальные файлы после создания Docker Secrets следует безопасно удалить.

## 5. Передать credentials в Docker Swarm

```bash
docker secret create vault_approle_role_id role-id.txt
docker secret create vault_approle_secret_id secret-id.txt
```

После этого используйте [stack.yaml](stack.yaml), где credentials монтируются как:

```text
/run/secrets/vault_approle_role_id
/run/secrets/vault_approle_secret_id
```

Injector перечитывает оба файла при каждом новом AppRole login. Для ротации Docker Secrets создайте новые версионированные secrets и перемонтируйте их под теми же target paths.

## 6. Проверить AppRole вручную

Для диагностической проверки можно выполнить login вручную:

```bash
vault write "auth/$AUTH_PATH/login" \
  role_id="$(tr -d '\n' < role-id.txt)" \
  secret_id="$(tr -d '\n' < secret-id.txt)"
```

В ответе должны присутствовать `token_duration`, равный примерно `20m`, и `token_renewable=true`.
````

### `examples/stack.yaml`

```yaml
version: "3.8"

# This stack contains the injector and five independent injection examples.
# Every `kv` label names a deliberately explicit KV v2 mount that an operator
# would create in Vault. Grant the injector policy read access to both the
# `metadata/` and `data/` paths of every mount used below.
#
# The example secret values shown in comments are illustrative. Do not store
# real credentials in this file.

services:
  injector:
    image: ghcr.io/melichron/docker-vault-injector:latest
    environment:
      VAULT_ADDR: "https://vault.example.com:8200"
      VAULT_AUTH_METHOD: "approle"
      # This is the full Vault API mount path, but without the /login suffix.
      VAULT_APPROLE_AUTH_PATH: "auth/docker-swarm"

      # # AppRole credentials may be passed directly. Files are
      # # preferred because the injector re-reads them before every login.
      # VAULT_APPROLE_ROLE_ID: "yyyyyyyy-gggg-1111-tttt-123456789abc"
      # VAULT_APPROLE_SECRET_ID: "xxxxxxxx-gggg-1111-tttt-123456789abc"
      VAULT_TOKEN_CHECK_INTERVAL: "30s"
      INJECTOR_POLL_INTERVAL: "30s"
      INJECTOR_LOG_LEVEL: "info"

      # File credentials are re-read before every AppRole login. This supports
      # rotation without restarting the injector when the mounted files are mutable.
      # Docker Swarm Secrets themselves are immutable and require a service update
      # with a newly created secret.
      VAULT_APPROLE_ROLE_ID_FILE: "/run/secrets/vault_approle_role_id"
      VAULT_APPROLE_SECRET_ID_FILE: "/run/secrets/vault_approle_secret_id"

    # # If mounted from filesystem, files can be updated without restarting injector service
    #   VAULT_APPROLE_ROLE_ID_FILE: "/vault-auth/role-id"
    #   VAULT_APPROLE_SECRET_ID_FILE: "/vault-auth/secret-id"
    # volumes:
    #   - /opt/docker-vault-injector/auth:/vault-auth:ro

    secrets:
      - vault_approle_role_id
      - vault_approle_secret_id
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    healthcheck:
      test: ["CMD", "docker-vault-injector", "health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    deploy:
      replicas: 1
      placement:
        constraints:
          - node.role==manager
      restart_policy:
        condition: any
        delay: 5s

  # Example 1: import one flat Vault document as-is.
  #
  # Mount: example-backend-kv
  # Path:  applications/backend/runtime
  # Data:
  #   {
  #     "BACKEND_LOG_LEVEL": "info",
  #     "BACKEND_PUBLIC_URL": "https://backend.example.com",
  #     "BACKEND_WORKERS": 4
  #   }
  #
  # No `.env.*` labels are present, so all three top-level scalar fields become
  # environment variables with exactly the same names.
  flat-secret-as-is:
    image: alpine:3.22
    command: ["sh", "-c", "while true; do sleep 3600; done"]
    deploy:
      replicas: 1
      labels:
        io.github.docker-vault-injector.enabled: "true"
        io.github.docker-vault-injector.secrets.backend-runtime.name: "backend-runtime-settings"
        io.github.docker-vault-injector.secrets.backend-runtime.kv: "example-backend-kv"
        io.github.docker-vault-injector.secrets.backend-runtime.vault-path: "applications/backend/runtime"
      update_config:
        order: start-first
        parallelism: 1
        failure_action: rollback

  # Example 2: combine two independent flat Vault documents in one service.
  #
  # Mount: example-shared-services-kv
  # Path:  environments/demo/common
  # Data:  {"TZ":"Europe/Kyiv","OTEL_SERVICE_NAME":"orders-api"}
  #
  # Mount: example-orders-database-kv
  # Path:  applications/orders/database
  # Data:  {"DB_HOST":"orders-db","DB_PORT":5432,"DB_USER":"orders"}
  #
  # Both documents are imported as-is. Their top-level keys must not overlap:
  # one environment variable may be owned by only one Vault source.
  two-flat-secrets:
    image: alpine:3.22
    command: ["sh", "-c", "while true; do sleep 3600; done"]
    deploy:
      replicas: 1
      labels:
        io.github.docker-vault-injector.enabled: "true"

        io.github.docker-vault-injector.secrets.shared-runtime.name: "shared-runtime-settings"
        io.github.docker-vault-injector.secrets.shared-runtime.kv: "example-shared-services-kv"
        io.github.docker-vault-injector.secrets.shared-runtime.vault-path: "environments/demo/common"

        io.github.docker-vault-injector.secrets.orders-database.name: "orders-database-connection"
        io.github.docker-vault-injector.secrets.orders-database.kv: "example-orders-database-kv"
        io.github.docker-vault-injector.secrets.orders-database.vault-path: "applications/orders/database"
      update_config:
        order: start-first
        parallelism: 1
        failure_action: rollback

  # Example 3: keep an initialization-sensitive service unscheduled until its
  # first successful injection.
  #
  # Mount: example-postgres-bootstrap-kv
  # Path:  databases/reporting/bootstrap
  # Data:
  #   {
  #     "POSTGRES_DB": "reporting",
  #     "POSTGRES_USER": "reporting_app",
  #     "POSTGRES_PASSWORD": "replace-with-a-real-secret"
  #   }
  #
  # Never add `io.github.docker-vault-injector.gate=open` to a Swarm node. The
  # impossible constraint leaves the task Pending. After resolving Vault data,
  # the injector installs POSTGRES_* and removes only that reserved constraint
  # in the same ServiceUpdate. Keep the gate label and constraint in this file
  # so later `docker stack deploy` operations are gated again.
  gated-postgres:
    image: postgres:17-alpine
    volumes:
      - postgres_example_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U \"$${POSTGRES_USER}\" -d \"$${POSTGRES_DB}\""]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 20s
    stop_grace_period: 60s
    deploy:
      replicas: 1
      placement:
        constraints:
          # A real application constraint is preserved after injection.
          - node.role==manager
          # This exact reserved constraint is removed after injection.
          - node.labels.io.github.docker-vault-injector.gate==open
      labels:
        io.github.docker-vault-injector.enabled: "true"
        io.github.docker-vault-injector.bootstrap-gate: "true"
        io.github.docker-vault-injector.secrets.postgres-bootstrap.name: "reporting-postgres-bootstrap"
        io.github.docker-vault-injector.secrets.postgres-bootstrap.kv: "example-postgres-bootstrap-kv"
        io.github.docker-vault-injector.secrets.postgres-bootstrap.vault-path: "databases/reporting/bootstrap"
      update_config:
        order: stop-first
        parallelism: 1
        failure_action: rollback
      restart_policy:
        condition: any
        delay: 5s

  # Example 4: import only selected top-level fields and rename them.
  #
  # Mount: example-payment-provider-kv
  # Path:  integrations/payment-provider/credentials
  # Data:
  #   {
  #     "client_id": "demo-client",
  #     "client_secret": "replace-with-a-real-secret",
  #     "operator_note": "this field must not enter the container"
  #   }
  #
  # Because `.env.*` labels are present, automatic import is disabled for this
  # source. Only client_id and client_secret are injected, under the explicit
  # PAYMENT_CLIENT_ID and PAYMENT_CLIENT_SECRET environment names.
  selected-top-level-fields:
    image: alpine:3.22
    command: ["sh", "-c", "while true; do sleep 3600; done"]
    deploy:
      replicas: 1
      labels:
        io.github.docker-vault-injector.enabled: "true"
        io.github.docker-vault-injector.secrets.payment-provider.name: "payment-provider-credentials"
        io.github.docker-vault-injector.secrets.payment-provider.kv: "example-payment-provider-kv"
        io.github.docker-vault-injector.secrets.payment-provider.vault-path: "integrations/payment-provider/credentials"
        io.github.docker-vault-injector.secrets.payment-provider.env.PAYMENT_CLIENT_ID: "client_id"
        io.github.docker-vault-injector.secrets.payment-provider.env.PAYMENT_CLIENT_SECRET: "client_secret"
      update_config:
        order: start-first
        parallelism: 1
        failure_action: rollback

  # Example 5: resolve selected scalar values from a nested JSON document.
  #
  # Mount: example-structured-application-kv
  # Path:  applications/notifications/structured-config
  # Data:
  #   {
  #     "smtp": {
  #       "endpoint": {"host":"smtp.example.com","port":587},
  #       "credentials": {"username":"mailer","password":"secret"}
  #     },
  #     "features": {"delivery_reports":true}
  #   }
  #
  # Label values are dotted paths inside the Vault JSON document. Only scalar
  # leaves can be injected; an object, array, or null value is rejected.
  selected-nested-json-fields:
    image: alpine:3.22
    command: ["sh", "-c", "while true; do sleep 3600; done"]
    deploy:
      replicas: 1
      labels:
        io.github.docker-vault-injector.enabled: "true"
        io.github.docker-vault-injector.secrets.notification-config.name: "notification-structured-config"
        io.github.docker-vault-injector.secrets.notification-config.kv: "example-structured-application-kv"
        io.github.docker-vault-injector.secrets.notification-config.vault-path: "applications/notifications/structured-config"
        io.github.docker-vault-injector.secrets.notification-config.env.SMTP_HOST: "smtp.endpoint.host"
        io.github.docker-vault-injector.secrets.notification-config.env.SMTP_PORT: "smtp.endpoint.port"
        io.github.docker-vault-injector.secrets.notification-config.env.SMTP_USERNAME: "smtp.credentials.username"
        io.github.docker-vault-injector.secrets.notification-config.env.SMTP_PASSWORD: "smtp.credentials.password"
        io.github.docker-vault-injector.secrets.notification-config.env.DELIVERY_REPORTS_ENABLED: "features.delivery_reports"
      update_config:
        order: start-first
        parallelism: 1
        failure_action: rollback

volumes:
  postgres_example_data:

secrets:
  vault_approle_role_id:
    external: true
  vault_approle_secret_id:
    external: true
```

### `examples/vault-policy.hcl`

```hcl
# The injector polls KV v2 metadata and reads an exact data version only when
# the version or the current Docker environment state has changed.
path "kv/metadata/apps/*" {
  capabilities = ["read"]
}

path "kv/data/apps/*" {
  capabilities = ["read"]
}

# These endpoints let the controller verify and renew the service token issued
# by AppRole. Vault's default policy normally grants them, but keeping them
# explicit makes this policy work if the role configuration changes later.
path "auth/token/lookup-self" {
  capabilities = ["read"]
}

path "auth/token/renew-self" {
  capabilities = ["update"]
}
```

### `go.mod`

```text
module github.com/melichron/docker-vault-injector

go 1.26

require (
	github.com/hashicorp/vault/api v1.23.0
	github.com/moby/moby/api v1.55.0
	github.com/moby/moby/client v0.5.1
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/go-connections v0.7.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-jose/go-jose/v4 v4.1.1 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.8 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-secure-stdlib/parseutil v0.2.0 // indirect
	github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 // indirect
	github.com/hashicorp/go-sockaddr v1.0.7 // indirect
	github.com/hashicorp/hcl v1.0.1-vault-7 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/ryanuber/go-glob v1.0.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.60.0 // indirect
	go.opentelemetry.io/otel v1.35.0 // indirect
	go.opentelemetry.io/otel/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/trace v1.35.0 // indirect
	golang.org/x/crypto v0.45.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	golang.org/x/time v0.12.0 // indirect
)
```

### `go.sum`

```text
github.com/Microsoft/go-winio v0.6.2 h1:F2VQgta7ecxGYO8k3ZZz3RS8fVIXVxONVUPlNERoyfY=
github.com/Microsoft/go-winio v0.6.2/go.mod h1:yd8OoFMLzJbo9gZq8j5qaps8bJ9aShtEA8Ipt1oGCvU=
github.com/cenkalti/backoff/v4 v4.3.0 h1:MyRJ/UdXutAwSAT+s3wNd7MfTIcy71VQueUuFK343L8=
github.com/cenkalti/backoff/v4 v4.3.0/go.mod h1:Y3VNntkOUPxTVeUxJ/G5vcM//AlwfmyYozVcomhLiZE=
github.com/containerd/errdefs v1.0.0 h1:tg5yIfIlQIrxYtu9ajqY42W3lpS19XqdxRQeEwYG8PI=
github.com/containerd/errdefs v1.0.0/go.mod h1:+YBYIdtsnF4Iw6nWZhJcqGSg/dwvV7tyJ/kCkyJ2k+M=
github.com/containerd/errdefs/pkg v0.3.0 h1:9IKJ06FvyNlexW690DXuQNx2KA2cUJXx151Xdx3ZPPE=
github.com/containerd/errdefs/pkg v0.3.0/go.mod h1:NJw6s9HwNuRhnjJhM7pylWwMyAkmCQvQ4GpJHEqRLVk=
github.com/davecgh/go-spew v1.1.1 h1:vj9j/u1bqnvCEfJOwUhtlOARqs3+rkHYY13jYWTU97c=
github.com/davecgh/go-spew v1.1.1/go.mod h1:J7Y8YcW2NihsgmVo/mv3lAwl/skON4iLHjSsI+c5H38=
github.com/distribution/reference v0.6.0 h1:0IXCQ5g4/QMHHkarYzh5l+u8T3t73zM5QvfrDyIgxBk=
github.com/distribution/reference v0.6.0/go.mod h1:BbU0aIcezP1/5jX/8MP0YiH4SdvB5Y4f/wlDRiLyi3E=
github.com/docker/go-connections v0.7.0 h1:6SsRfJddP22WMrCkj19x9WKjEDTB+ahsdiGYf0mN39c=
github.com/docker/go-connections v0.7.0/go.mod h1:no1qkHdjq7kLMGUXYAduOhYPSJxxvgWBh7ogVvptn3Q=
github.com/docker/go-units v0.5.0 h1:69rxXcBk27SvSaaxTtLh/8llcHD8vYHT7WSdRZ/jvr4=
github.com/docker/go-units v0.5.0/go.mod h1:fgPhTUdO+D/Jk86RDLlptpiXQzgHJF7gydDDbaIK4Dk=
github.com/fatih/color v1.18.0 h1:S8gINlzdQ840/4pfAwic/ZE0djQEH3wM94VfqLTZcOM=
github.com/fatih/color v1.18.0/go.mod h1:4FelSpRwEGDpQ12mAdzqdOukCy4u8WUtOY6lkT/6HfU=
github.com/felixge/httpsnoop v1.0.4 h1:NFTV2Zj1bL4mc9sqWACXbQFVBBg2W3GPvqp8/ESS2Wg=
github.com/felixge/httpsnoop v1.0.4/go.mod h1:m8KPJKqk1gH5J9DgRY2ASl2lWCfGKXixSwevea8zH2U=
github.com/go-jose/go-jose/v4 v4.1.1 h1:JYhSgy4mXXzAdF3nUx3ygx347LRXJRrpgyU3adRmkAI=
github.com/go-jose/go-jose/v4 v4.1.1/go.mod h1:BdsZGqgdO3b6tTc6LSE56wcDbMMLuPsw5d4ZD5f94kA=
github.com/go-logr/logr v1.2.2/go.mod h1:jdQByPbusPIv2/zmleS9BjJVeZ6kBagPoEUsqbVz/1A=
github.com/go-logr/logr v1.4.2 h1:6pFjapn8bFcIbiKo3XT4j/BhANplGihG6tvd+8rYgrY=
github.com/go-logr/logr v1.4.2/go.mod h1:9T104GzyrTigFIr8wt5mBrctHMim0Nb2HLGrmQ40KvY=
github.com/go-logr/stdr v1.2.2 h1:hSWxHoqTgW2S2qGc0LTAI563KZ5YKYRhT3MFKZMbjag=
github.com/go-logr/stdr v1.2.2/go.mod h1:mMo/vtBO5dYbehREoey6XUKy/eSumjCCveDpRre4VKE=
github.com/go-test/deep v1.1.1 h1:0r/53hagsehfO4bzD2Pgr/+RgHqhmf+k1Bpse2cTu1U=
github.com/go-test/deep v1.1.1/go.mod h1:5C2ZWiW0ErCdrYzpqxLbTX7MG14M9iiw8DgHncVwcsE=
github.com/google/go-cmp v0.7.0 h1:wk8382ETsv4JYUZwIsn6YpYiWiBsYLSJiTsyBybVuN8=
github.com/google/go-cmp v0.7.0/go.mod h1:pXiqmnSA92OHEEa9HXL2W4E7lf9JzCmGVUdgjX3N/iU=
github.com/google/uuid v1.6.0 h1:NIvaJDMOsjHA8n1jAhLSgzrAzy1Hgr+hNrb57e+94F0=
github.com/google/uuid v1.6.0/go.mod h1:TIyPZe4MgqvfeYDBFedMoGGpEw/LqOeaOT+nhxU+yHo=
github.com/hashicorp/errwrap v1.0.0/go.mod h1:YH+1FKiLXxHSkmPseP+kNlulaMuP3n2brvKWEqk/Jc4=
github.com/hashicorp/errwrap v1.1.0 h1:OxrOeh75EUXMY8TBjag2fzXGZ40LB6IKw45YeGUDY2I=
github.com/hashicorp/errwrap v1.1.0/go.mod h1:YH+1FKiLXxHSkmPseP+kNlulaMuP3n2brvKWEqk/Jc4=
github.com/hashicorp/go-cleanhttp v0.5.2 h1:035FKYIWjmULyFRBKPs8TBQoi0x6d9G4xc9neXJWAZQ=
github.com/hashicorp/go-cleanhttp v0.5.2/go.mod h1:kO/YDlP8L1346E6Sodw+PrpBSV4/SoxCXGY6BqNFT48=
github.com/hashicorp/go-hclog v1.6.3 h1:Qr2kF+eVWjTiYmU7Y31tYlP1h0q/X3Nl3tPGdaB11/k=
github.com/hashicorp/go-hclog v1.6.3/go.mod h1:W4Qnvbt70Wk/zYJryRzDRU/4r0kIg0PVHBcfoyhpF5M=
github.com/hashicorp/go-multierror v1.1.1 h1:H5DkEtf6CXdFp0N0Em5UCwQpXMWke8IA0+lD48awMYo=
github.com/hashicorp/go-multierror v1.1.1/go.mod h1:iw975J/qwKPdAO1clOe2L8331t/9/fmwbPZ6JB6eMoM=
github.com/hashicorp/go-retryablehttp v0.7.8 h1:ylXZWnqa7Lhqpk0L1P1LzDtGcCR0rPVUrx/c8Unxc48=
github.com/hashicorp/go-retryablehttp v0.7.8/go.mod h1:rjiScheydd+CxvumBsIrFKlx3iS0jrZ7LvzFGFmuKbw=
github.com/hashicorp/go-rootcerts v1.0.2 h1:jzhAVGtqPKbwpyCPELlgNWhE1znq+qwJtW5Oi2viEzc=
github.com/hashicorp/go-rootcerts v1.0.2/go.mod h1:pqUvnprVnM5bf7AOirdbb01K4ccR319Vf4pU3K5EGc8=
github.com/hashicorp/go-secure-stdlib/parseutil v0.2.0 h1:U+kC2dOhMFQctRfhK0gRctKAPTloZdMU5ZJxaesJ/VM=
github.com/hashicorp/go-secure-stdlib/parseutil v0.2.0/go.mod h1:Ll013mhdmsVDuoIXVfBtvgGJsXDYkTw1kooNcoCXuE0=
github.com/hashicorp/go-secure-stdlib/strutil v0.1.2 h1:kes8mmyCpxJsI7FTwtzRqEy9CdjCtrXrXGuOpxEA7Ts=
github.com/hashicorp/go-secure-stdlib/strutil v0.1.2/go.mod h1:Gou2R9+il93BqX25LAKCLuM+y9U2T4hlwvT1yprcna4=
github.com/hashicorp/go-sockaddr v1.0.7 h1:G+pTkSO01HpR5qCxg7lxfsFEZaG+C0VssTy/9dbT+Fw=
github.com/hashicorp/go-sockaddr v1.0.7/go.mod h1:FZQbEYa1pxkQ7WLpyXJ6cbjpT8q0YgQaK/JakXqGyWw=
github.com/hashicorp/hcl v1.0.1-vault-7 h1:ag5OxFVy3QYTFTJODRzTKVZ6xvdfLLCA1cy/Y6xGI0I=
github.com/hashicorp/hcl v1.0.1-vault-7/go.mod h1:XYhtn6ijBSAj6n4YqAaf7RBPS4I06AItNorpy+MoQNM=
github.com/hashicorp/vault/api v1.23.0 h1:gXgluBsSECfRWTSW9niY2jwg2e9mMJc4WoHNv4g3h6A=
github.com/hashicorp/vault/api v1.23.0/go.mod h1:zransKiB9ftp+kgY8ydjnvCU7Wk8i9L0DYWpXeMj9ko=
github.com/kr/pretty v0.3.1 h1:flRD4NNwYAUpkphVc1HcthR4KEIFJ65n8Mw5qdRn3LE=
github.com/kr/pretty v0.3.1/go.mod h1:hoEshYVHaxMs3cyo3Yncou5ZscifuDolrwPKZanG3xk=
github.com/kr/text v0.2.0 h1:5Nx0Ya0ZqY2ygV366QzturHI13Jq95ApcVaJBhpS+AY=
github.com/kr/text v0.2.0/go.mod h1:eLer722TekiGuMkidMxC/pM04lWEeraHUUmBw8l2grE=
github.com/mattn/go-colorable v0.1.14 h1:9A9LHSqF/7dyVVX6g0U9cwm9pG3kP9gSzcuIPHPsaIE=
github.com/mattn/go-colorable v0.1.14/go.mod h1:6LmQG8QLFO4G5z1gPvYEzlUgJ2wF+stgPZH1UqBm1s8=
github.com/mattn/go-isatty v0.0.20 h1:xfD0iDuEKnDkl03q4limB+vH+GxLEtL/jb4xVJSWWEY=
github.com/mattn/go-isatty v0.0.20/go.mod h1:W+V8PltTTMOvKvAeJH7IuucS94S2C6jfK/D7dTCTo3Y=
github.com/mitchellh/go-homedir v1.1.0 h1:lukF9ziXFxDFPkA1vsr5zpc1XuPDn/wFntq5mG+4E0Y=
github.com/mitchellh/go-homedir v1.1.0/go.mod h1:SfyaCUpYCn1Vlf4IUYiD9fPX4A5wJrkLzIz1N1q0pr0=
github.com/mitchellh/mapstructure v1.5.0 h1:jeMsZIYE/09sWLaz43PL7Gy6RuMjD2eJVyuac5Z2hdY=
github.com/mitchellh/mapstructure v1.5.0/go.mod h1:bFUtVrKA4DC2yAKiSyO/QUcy7e+RRV2QTWOzhPopBRo=
github.com/moby/docker-image-spec v1.3.1 h1:jMKff3w6PgbfSa69GfNg+zN/XLhfXJGnEx3Nl2EsFP0=
github.com/moby/docker-image-spec v1.3.1/go.mod h1:eKmb5VW8vQEh/BAr2yvVNvuiJuY6UIocYsFu/DxxRpo=
github.com/moby/moby/api v1.55.0 h1:2/sexvQyqIWS8pRSCFddBfpW2qE7vR7FCL+vN8pxwMc=
github.com/moby/moby/api v1.55.0/go.mod h1:+RQ6wluLwtYaTd1WnPLykIDPekkuyD/ROWQClE83pzs=
github.com/moby/moby/client v0.5.1 h1:tYNaJno4c0HXz12y5BiqEDy0rVTYkWzI26lGvnTMiJw=
github.com/moby/moby/client v0.5.1/go.mod h1:odLstlZ6uSnfvAgVxMpvgmb8SUdd+siH2T0GBuxVAlM=
github.com/opencontainers/go-digest v1.0.0 h1:apOUWs51W5PlhuyGyz9FCeeBIOUDA/6nW8Oi/yOhh5U=
github.com/opencontainers/go-digest v1.0.0/go.mod h1:0JzlMkj0TRzQZfJkVvzbP0HBR3IKzErnv2BNG4W4MAM=
github.com/opencontainers/image-spec v1.1.1 h1:y0fUlFfIZhPF1W537XOLg0/fcx6zcHCJwooC2xJA040=
github.com/opencontainers/image-spec v1.1.1/go.mod h1:qpqAh3Dmcf36wStyyWU+kCeDgrGnAve2nCC8+7h8Q0M=
github.com/pmezard/go-difflib v1.0.0 h1:4DBwDE0NGyQoBHbLQYPwSUPoCMWR5BEzIk/f1lZbAQM=
github.com/pmezard/go-difflib v1.0.0/go.mod h1:iKH77koFhYxTK1pcRnkKkqfTogsbg7gZNVY4sRDYZ/4=
github.com/rogpeppe/go-internal v1.13.1 h1:KvO1DLK/DRN07sQ1LQKScxyZJuNnedQ5/wKSR38lUII=
github.com/rogpeppe/go-internal v1.13.1/go.mod h1:uMEvuHeurkdAXX61udpOXGD/AzZDWNMNyH2VO9fmH0o=
github.com/ryanuber/go-glob v1.0.0 h1:iQh3xXAumdQ+4Ufa5b25cRpC5TYKlno6hsv6Cb3pkBk=
github.com/ryanuber/go-glob v1.0.0/go.mod h1:807d1WSdnB0XRJzKNil9Om6lcp/3a0v4qIHxIXzX/Yc=
github.com/stretchr/testify v1.10.0 h1:Xv5erBjTwe/5IxqUQTdXv5kgmIvbHo3QQyRwhJsOfJA=
github.com/stretchr/testify v1.10.0/go.mod h1:r2ic/lqez/lEtzL7wO/rwa5dbSLXVDPFyf8C91i36aY=
go.opentelemetry.io/auto/sdk v1.1.0 h1:cH53jehLUN6UFLY71z+NDOiNJqDdPRaXzTel0sJySYA=
go.opentelemetry.io/auto/sdk v1.1.0/go.mod h1:3wSPjt5PWp2RhlCcmmOial7AvC4DQqZb7a7wCow3W8A=
go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.60.0 h1:sbiXRNDSWJOTobXh5HyQKjq6wUC5tNybqjIqDpAY4CU=
go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.60.0/go.mod h1:69uWxva0WgAA/4bu2Yy70SLDBwZXuQ6PbBpbsa5iZrQ=
go.opentelemetry.io/otel v1.35.0 h1:xKWKPxrxB6OtMCbmMY021CqC45J+3Onta9MqjhnusiQ=
go.opentelemetry.io/otel v1.35.0/go.mod h1:UEqy8Zp11hpkUrL73gSlELM0DupHoiq72dR+Zqel/+Y=
go.opentelemetry.io/otel/metric v1.35.0 h1:0znxYu2SNyuMSQT4Y9WDWej0VpcsxkuklLa4/siN90M=
go.opentelemetry.io/otel/metric v1.35.0/go.mod h1:nKVFgxBZ2fReX6IlyW28MgZojkoAkJGaE8CpgeAU3oE=
go.opentelemetry.io/otel/sdk v1.35.0 h1:iPctf8iprVySXSKJffSS79eOjl9pvxV9ZqOWT0QejKY=
go.opentelemetry.io/otel/sdk v1.35.0/go.mod h1:+ga1bZliga3DxJ3CQGg3updiaAJoNECOgJREo9KHGQg=
go.opentelemetry.io/otel/sdk/metric v1.35.0 h1:1RriWBmCKgkeHEhM7a2uMjMUfP7MsOF5JpUCaEqEI9o=
go.opentelemetry.io/otel/sdk/metric v1.35.0/go.mod h1:is6XYCUMpcKi+ZsOvfluY5YstFnhW0BidkR+gL+qN+w=
go.opentelemetry.io/otel/trace v1.35.0 h1:dPpEfJu1sDIqruz7BHFG3c7528f6ddfSWfFDVt/xgMs=
go.opentelemetry.io/otel/trace v1.35.0/go.mod h1:WUk7DtFp1Aw2MkvqGdwiXYDZZNvA/1J8o6xRXLrIkyc=
golang.org/x/crypto v0.45.0 h1:jMBrvKuj23MTlT0bQEOBcAE0mjg8mK9RXFhRH6nyF3Q=
golang.org/x/crypto v0.45.0/go.mod h1:XTGrrkGJve7CYK7J8PEww4aY7gM3qMCElcJQ8n8JdX4=
golang.org/x/net v0.47.0 h1:Mx+4dIFzqraBXUugkia1OOvlD6LemFo1ALMHjrXDOhY=
golang.org/x/net v0.47.0/go.mod h1:/jNxtkgq5yWUGYkaZGqo27cfGZ1c5Nen03aYrrKpVRU=
golang.org/x/sys v0.38.0 h1:3yZWxaJjBmCWXqhN1qh02AkOnCQ1poK6oF+a7xWL6Gc=
golang.org/x/sys v0.38.0/go.mod h1:OgkHotnGiDImocRcuBABYBEXf8A9a87e/uXjp9XT3ks=
golang.org/x/text v0.31.0 h1:aC8ghyu4JhP8VojJ2lEHBnochRno1sgL6nEi9WGFGMM=
golang.org/x/text v0.31.0/go.mod h1:tKRAlv61yKIjGGHX/4tP1LTbc13YSec1pxVEWXzfoeM=
golang.org/x/time v0.12.0 h1:ScB/8o8olJvc+CQPWrK3fPZNfh7qgwCrY0zJmoEQLSE=
golang.org/x/time v0.12.0/go.mod h1:CDIdPxbZBQxdj6cxyCIdrNogrJKMJ7pr37NYpMcMDSg=
gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405/go.mod h1:Co6ibVJAznAaIkqp8huTwlJQCZ016jof/cbN4VW5Yz0=
gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c h1:Hei/4ADfdWqJk1ZMxUNpqntNwaWcugrBjAiHlqqRiVk=
gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c/go.mod h1:JHkPIbrfpd72SG/EVd6muEfDQjcINNoR0C8j2r3qZ4Q=
gopkg.in/yaml.v3 v3.0.1 h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=
gopkg.in/yaml.v3 v3.0.1/go.mod h1:K4uyk7z7BCEPqu6E+C64Yfv1cQ7kz7rIZviUmN+EgEM=
gotest.tools/v3 v3.5.2 h1:7koQfIKdy+I8UTetycgUqXWSDwpgv193Ka+qRsmBY8Q=
gotest.tools/v3 v3.5.2/go.mod h1:LtdLGcnqToBH83WByAAi/wiwSFCArdFIUV/xxN4pcjA=
pgregory.net/rapid v1.2.0 h1:keKAYRcjm+e1F0oAuU5F5+YPAWcyxNNRK2wud503Gnk=
pgregory.net/rapid v1.2.0/go.mod h1:PY5XlDGj0+V1FCq0o192FdRhpKHGTRIWBgqjDBTrq04=
```

### `internal/config/config_test.go`

```go
package config

import (
	"testing"
	"time"
)

func TestLoadRequiresExplicitAppRoleAuthPath(t *testing.T) {
	t.Setenv("VAULT_AUTH_METHOD", "approle")
	t.Setenv("VAULT_APPROLE_AUTH_PATH", "")
	if _, err := Load(); err == nil {
		t.Fatal("AppRole configuration without VAULT_APPROLE_AUTH_PATH should fail")
	}
}

func TestLoadAllowsExplicitTokenFallback(t *testing.T) {
	t.Setenv("VAULT_AUTH_METHOD", "token")
	t.Setenv("VAULT_APPROLE_AUTH_PATH", "")
	configuration, err := Load()
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}
	if configuration.VaultAuth.Method != "token" {
		t.Fatalf("auth method = %q", configuration.VaultAuth.Method)
	}
}

func TestStatusFileFromEnvironment(t *testing.T) {
	t.Setenv("INJECTOR_STATUS_FILE", "/run/injector/status.json")
	if got := StatusFileFromEnvironment(); got != "/run/injector/status.json" {
		t.Fatalf("status file = %q", got)
	}

	t.Setenv("INJECTOR_STATUS_FILE", "")
	if got := StatusFileFromEnvironment(); got != DefaultStatusFile {
		t.Fatalf("default status file = %q", got)
	}
}

func TestHealthMaxAgeFromEnvironment(t *testing.T) {
	t.Setenv("INJECTOR_HEALTH_MAX_AGE", "3m")
	if got, err := HealthMaxAgeFromEnvironment(); err != nil || got != 3*time.Minute {
		t.Fatalf("health max age = %s, err = %v", got, err)
	}

	t.Setenv("INJECTOR_HEALTH_MAX_AGE", "")
	if got, err := HealthMaxAgeFromEnvironment(); err != nil || got != DefaultHealthMaxAge {
		t.Fatalf("default health max age = %s, err = %v", got, err)
	}
}

func TestLoadRejectsRetryMaximumBelowInitialInterval(t *testing.T) {
	t.Setenv("VAULT_APPROLE_AUTH_PATH", "auth/swarm")
	t.Setenv("INJECTOR_EVENT_RETRY_INTERVAL", "10s")
	t.Setenv("INJECTOR_EVENT_RETRY_MAX_INTERVAL", "5s")
	if _, err := Load(); err == nil {
		t.Fatal("expected event retry maximum validation error")
	}
}
```

### `internal/config/config.go`

```go
// Package config loads process-level configuration. Service-specific
// configuration belongs in Swarm labels and is parsed by internal/labels.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	DefaultStatusFile   = "/tmp/docker-vault-injector-status.json"
	DefaultHealthMaxAge = 80 * time.Second
)

type Config struct {
	PollInterval       time.Duration
	ReconcileTimeout   time.Duration
	EventRetryInterval time.Duration
	EventRetryMaximum  time.Duration
	HealthMaxAge       time.Duration
	StatusFile         string
	VaultAuth          VaultAuthConfig
	LogLevel           slog.Level
}

// VaultAuthConfig contains authentication settings only. The Vault address,
// namespace and TLS settings continue to use the standard VAULT_* variables
// understood by HashiCorp's official Go client.
type VaultAuthConfig struct {
	Method              string
	AppRoleAuthPath     string
	AppRoleRoleID       string
	AppRoleRoleIDFile   string
	AppRoleSecretID     string
	AppRoleSecretIDFile string
	Token               string
	TokenFile           string
	TokenCheckInterval  time.Duration
	AuthRetryInterval   time.Duration
	AuthRetryMaximum    time.Duration
}

func Load() (Config, error) {
	pollInterval, err := durationFromEnvironment("INJECTOR_POLL_INTERVAL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	reconcileTimeout, err := durationFromEnvironment("INJECTOR_RECONCILE_TIMEOUT", 20*time.Second)
	if err != nil {
		return Config{}, err
	}
	eventRetryInterval, err := durationFromEnvironment("INJECTOR_EVENT_RETRY_INTERVAL", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	eventRetryMaximum, err := durationFromEnvironment("INJECTOR_EVENT_RETRY_MAX_INTERVAL", time.Minute)
	if err != nil {
		return Config{}, err
	}
	healthMaxAge, err := HealthMaxAgeFromEnvironment()
	if err != nil {
		return Config{}, err
	}
	tokenCheckInterval, err := durationFromEnvironment("VAULT_TOKEN_CHECK_INTERVAL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	authRetryInterval, err := durationFromEnvironment("VAULT_AUTH_RETRY_INTERVAL", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	authRetryMaximum, err := durationFromEnvironment("VAULT_AUTH_RETRY_MAX_INTERVAL", time.Minute)
	if err != nil {
		return Config{}, err
	}

	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("INJECTOR_LOG_LEVEL")) {
	case "", "info":
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return Config{}, fmt.Errorf("INJECTOR_LOG_LEVEL must be debug, info, warn, or error")
	}

	if pollInterval <= 0 || reconcileTimeout <= 0 || eventRetryInterval <= 0 ||
		eventRetryMaximum <= 0 || healthMaxAge <= 0 || tokenCheckInterval <= 0 ||
		authRetryInterval <= 0 || authRetryMaximum <= 0 {
		return Config{}, fmt.Errorf("all duration settings must be greater than zero")
	}
	if eventRetryMaximum < eventRetryInterval {
		return Config{}, fmt.Errorf("INJECTOR_EVENT_RETRY_MAX_INTERVAL must be greater than or equal to INJECTOR_EVENT_RETRY_INTERVAL")
	}
	if authRetryMaximum < authRetryInterval {
		return Config{}, fmt.Errorf("VAULT_AUTH_RETRY_MAX_INTERVAL must be greater than or equal to VAULT_AUTH_RETRY_INTERVAL")
	}

	authMethod := strings.ToLower(strings.TrimSpace(os.Getenv("VAULT_AUTH_METHOD")))
	if authMethod == "" {
		authMethod = "approle"
	}
	if authMethod != "approle" && authMethod != "token" {
		return Config{}, fmt.Errorf("VAULT_AUTH_METHOD must be approle or token")
	}
	if authMethod == "approle" && strings.TrimSpace(os.Getenv("VAULT_APPROLE_AUTH_PATH")) == "" {
		return Config{}, fmt.Errorf("VAULT_APPROLE_AUTH_PATH is required for AppRole authentication")
	}

	return Config{
		PollInterval:       pollInterval,
		ReconcileTimeout:   reconcileTimeout,
		EventRetryInterval: eventRetryInterval,
		EventRetryMaximum:  eventRetryMaximum,
		HealthMaxAge:       healthMaxAge,
		StatusFile:         StatusFileFromEnvironment(),
		VaultAuth: VaultAuthConfig{
			Method:              authMethod,
			AppRoleAuthPath:     os.Getenv("VAULT_APPROLE_AUTH_PATH"),
			AppRoleRoleID:       os.Getenv("VAULT_APPROLE_ROLE_ID"),
			AppRoleRoleIDFile:   os.Getenv("VAULT_APPROLE_ROLE_ID_FILE"),
			AppRoleSecretID:     os.Getenv("VAULT_APPROLE_SECRET_ID"),
			AppRoleSecretIDFile: os.Getenv("VAULT_APPROLE_SECRET_ID_FILE"),
			Token:               os.Getenv("VAULT_TOKEN"),
			TokenFile:           os.Getenv("VAULT_TOKEN_FILE"),
			TokenCheckInterval:  tokenCheckInterval,
			AuthRetryInterval:   authRetryInterval,
			AuthRetryMaximum:    authRetryMaximum,
		},
		LogLevel: level,
	}, nil
}

// StatusFileFromEnvironment is intentionally independent from Load so the
// status subcommands can read the running controller's snapshot without
// requiring Docker or Vault authentication settings.
func StatusFileFromEnvironment() string {
	if value := strings.TrimSpace(os.Getenv("INJECTOR_STATUS_FILE")); value != "" {
		return value
	}
	return DefaultStatusFile
}

// HealthMaxAgeFromEnvironment remains independent from Load for the same
// reason as StatusFileFromEnvironment: the health subprocess must not require
// Vault credentials. The default covers two normal polling periods plus one
// reconciliation timeout.
func HealthMaxAgeFromEnvironment() (time.Duration, error) {
	return durationFromEnvironment("INJECTOR_HEALTH_MAX_AGE", DefaultHealthMaxAge)
}

func durationFromEnvironment(name string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return value, nil
}
```

### `internal/controller/controller_test.go`

```go
package controller

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/swarm"

	"github.com/melichron/docker-vault-injector/internal/labels"
	statusview "github.com/melichron/docker-vault-injector/internal/status"
	"github.com/melichron/docker-vault-injector/internal/vaultclient"
)

type fakeDocker struct {
	updates  []swarm.Service
	warnings []string
}

func (d *fakeDocker) ListServices(context.Context) ([]swarm.Service, error) {
	return nil, nil
}

func (d *fakeDocker) InspectService(context.Context, string) (swarm.Service, error) {
	return swarm.Service{}, nil
}

func (d *fakeDocker) UpdateService(_ context.Context, service swarm.Service) ([]string, error) {
	d.updates = append(d.updates, service)
	return d.warnings, nil
}

func (d *fakeDocker) WatchServiceEvents(context.Context) (<-chan events.Message, <-chan error) {
	messages := make(chan events.Message)
	errorsChannel := make(chan error)
	close(messages)
	close(errorsChannel)
	return messages, errorsChannel
}

type fakeVault struct {
	versions            map[string]int
	secrets             map[string]vaultclient.Secret
	currentVersionError error
	reads               int
}

func (v *fakeVault) CurrentVersion(_ context.Context, mount, path string) (int, error) {
	if v.currentVersionError != nil {
		return 0, v.currentVersionError
	}
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

func TestStatusSnapshotRecordsResultsWithoutSecretValues(t *testing.T) {
	statusPath := filepath.Join(t.TempDir(), "status.json")
	statusStore, err := statusview.NewStore(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	docker := &fakeDocker{warnings: []string{"registry auth was not updated"}}
	vault := &fakeVault{
		versions: map[string]int{"kv/apps/api": 7},
		secrets: map[string]vaultclient.Secret{
			"kv/apps/api": {Version: 7, Data: map[string]any{
				"username": "api-user",
				"database": map[string]any{"password": "correct horse"},
			}},
		},
	}
	controller := testControllerWithStatus(docker, vault, statusStore)

	if err := controller.ReconcileService(context.Background(), managedService()); err != nil {
		t.Fatal(err)
	}
	rawSnapshot, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"api-user", "correct horse"} {
		if strings.Contains(string(rawSnapshot), forbidden) {
			t.Fatalf("status snapshot contains secret value %q", forbidden)
		}
	}
	snapshot, err := statusview.Read(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Services) != 1 {
		t.Fatalf("status services = %d, want 1", len(snapshot.Services))
	}
	serviceStatus := snapshot.Services[0]
	if serviceStatus.State != statusview.StateReady {
		t.Fatalf("state = %q, want ready", serviceStatus.State)
	}
	if !slices.Equal(serviceStatus.EnvironmentNames, []string{"DB_PASSWORD", "DB_USER"}) {
		t.Fatalf("environment names = %v", serviceStatus.EnvironmentNames)
	}
	if serviceStatus.Versions["database"] != 7 || serviceStatus.LastSuccess == nil {
		t.Fatalf("unexpected successful status: %#v", serviceStatus)
	}
	if !slices.Equal(serviceStatus.Warnings, []string{"registry auth was not updated"}) {
		t.Fatalf("Docker warnings = %v", serviceStatus.Warnings)
	}

	vault.currentVersionError = errors.New("Vault unavailable")
	if err := controller.ReconcileService(context.Background(), docker.updates[0]); err == nil {
		t.Fatal("expected Vault error")
	}
	snapshot, err = statusview.Read(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	serviceStatus = snapshot.Services[0]
	if serviceStatus.State != statusview.StateError || !strings.Contains(serviceStatus.Error, "Vault unavailable") {
		t.Fatalf("unexpected error status: %#v", serviceStatus)
	}
	if serviceStatus.LastSuccess == nil {
		t.Fatal("failed reconciliation erased the last successful timestamp")
	}
	if !slices.Equal(serviceStatus.Warnings, []string{"registry auth was not updated"}) {
		t.Fatalf("failed reconciliation erased the last Docker warnings: %v", serviceStatus.Warnings)
	}
}

func TestBootstrapGateIsRemovedAtomicallyWithEnvironmentInjection(t *testing.T) {
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
	service := managedService()
	service.Spec.Labels[labels.BootstrapGateLabel] = "true"
	service.Spec.TaskTemplate.Placement = &swarm.Placement{Constraints: []string{
		"node.role==manager",
		"node.labels.io.github.docker-vault-injector.gate == open",
	}}

	if err := testController(docker, vault).ReconcileService(context.Background(), service); err != nil {
		t.Fatal(err)
	}
	if len(docker.updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(docker.updates))
	}
	updated := docker.updates[0]
	if hasBootstrapGate(updated) {
		t.Fatal("the successful update must remove the reserved bootstrap gate")
	}
	wantConstraints := []string{"node.role==manager"}
	if got := updated.Spec.TaskTemplate.Placement.Constraints; !slices.Equal(got, wantConstraints) {
		t.Fatalf("constraints = %v, want %v", got, wantConstraints)
	}
	if !hasBootstrapGate(service) {
		t.Fatal("reconciliation mutated the inspected service instead of its clone")
	}
	if got := updated.Spec.TaskTemplate.ContainerSpec.Env; !slices.Contains(got, "DB_PASSWORD=correct horse") {
		t.Fatalf("gate was removed without the desired environment: %v", got)
	}

	if err := testController(docker, vault).ReconcileService(context.Background(), updated); err != nil {
		t.Fatalf("reconcile after opening the gate failed: %v", err)
	}
	if len(docker.updates) != 1 {
		t.Fatal("the opened, current service caused an unnecessary update")
	}
	regated := cloneServiceForUpdate(updated)
	regated.Spec.TaskTemplate.Placement.Constraints = append(
		regated.Spec.TaskTemplate.Placement.Constraints,
		labels.BootstrapGateConstraint,
	)
	if err := testController(docker, vault).ReconcileService(context.Background(), regated); err != nil {
		t.Fatalf("reconcile after stack reapplied only the gate failed: %v", err)
	}
	if len(docker.updates) != 2 || hasBootstrapGate(docker.updates[1]) {
		t.Fatal("a reapplied gate must be removed even when environment and state are current")
	}
	updated = docker.updates[1]

	vault.versions["kv/apps/api"] = 8
	vault.secrets["kv/apps/api"] = vaultclient.Secret{Version: 8, Data: map[string]any{
		"username": "rotated-user",
		"database": map[string]any{"password": "rotated password"},
	}}
	if err := testController(docker, vault).ReconcileService(context.Background(), updated); err != nil {
		t.Fatalf("ordinary Vault rotation after opening the gate failed: %v", err)
	}
	if len(docker.updates) != 3 {
		t.Fatalf("Vault rotation updates = %d, want 3 total", len(docker.updates))
	}
	if hasBootstrapGate(docker.updates[2]) {
		t.Fatal("ordinary Vault rotation must not restore the bootstrap gate")
	}
}

func TestBootstrapGateFailsClosedWhenVaultIsUnavailable(t *testing.T) {
	docker := &fakeDocker{}
	vault := &fakeVault{currentVersionError: errors.New("Vault unavailable")}
	service := managedService()
	service.Spec.Labels[labels.BootstrapGateLabel] = "true"
	service.Spec.TaskTemplate.Placement = &swarm.Placement{Constraints: []string{
		labels.BootstrapGateConstraint,
	}}

	err := testController(docker, vault).ReconcileService(context.Background(), service)
	if err == nil {
		t.Fatal("Vault failure should fail reconciliation")
	}
	if len(docker.updates) != 0 {
		t.Fatal("Vault failure must not remove the gate or update the service")
	}
	if !hasBootstrapGate(service) {
		t.Fatal("Vault failure removed the gate from the inspected service")
	}
}

func TestBootstrapGateRequiresReservedConstraintBeforeFirstInjection(t *testing.T) {
	docker := &fakeDocker{}
	vault := &fakeVault{}
	service := managedService()
	service.Spec.Labels[labels.BootstrapGateLabel] = "true"

	err := testController(docker, vault).ReconcileService(context.Background(), service)
	if err == nil {
		t.Fatal("bootstrap-gate without its placement constraint should fail")
	}
	if len(docker.updates) != 0 {
		t.Fatal("misconfigured bootstrap gate must not update the service")
	}
}

func TestBootstrapGateRequiresConstraintWhenConfigurationChanges(t *testing.T) {
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
	service := managedService()
	service.Spec.Labels[labels.BootstrapGateLabel] = "true"
	service.Spec.TaskTemplate.Placement = &swarm.Placement{Constraints: []string{
		labels.BootstrapGateConstraint,
	}}
	controller := testController(docker, vault)

	if err := controller.ReconcileService(context.Background(), service); err != nil {
		t.Fatal(err)
	}
	updated := docker.updates[0]
	updated.Spec.Labels[labels.SecretsPrefix+"database.vault-path"] = "apps/changed"

	err := controller.ReconcileService(context.Background(), updated)
	if err == nil {
		t.Fatal("a gated configuration change without a restored constraint should fail")
	}
	if len(docker.updates) != 1 {
		t.Fatal("unsafe configuration change must not update the service")
	}
}

func TestReservedConstraintRequiresBootstrapGateLabel(t *testing.T) {
	docker := &fakeDocker{}
	service := managedService()
	service.Spec.TaskTemplate.Placement = &swarm.Placement{Constraints: []string{
		labels.BootstrapGateConstraint,
	}}

	err := testController(docker, &fakeVault{}).ReconcileService(context.Background(), service)
	if err == nil {
		t.Fatal("the reserved constraint must not be removed without explicit opt-in")
	}
	if len(docker.updates) != 0 {
		t.Fatal("reserved constraint without opt-in must remain untouched")
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
	service.Spec.TaskTemplate.Placement = &swarm.Placement{Constraints: []string{
		labels.BootstrapGateConstraint,
	}}
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
	if !hasBootstrapGate(updated) {
		t.Fatal("disabling injection must not remove a bootstrap gate")
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
	return testControllerWithStatus(docker, vault, nil)
}

func testControllerWithStatus(docker Docker, vault Vault, statusStore *statusview.Store) *Controller {
	return New(
		docker,
		vault,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		time.Minute,
		time.Minute,
		time.Second,
		time.Minute,
		statusStore,
	)
}
```

### `internal/controller/controller.go`

```go
// Package controller contains the reconciliation loop. It follows the same
// basic model as controllers in Kubernetes and tools such as Traefik:
// observe current state, calculate desired state, and make the smallest update
// needed to converge the two.
package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/swarm"

	"github.com/melichron/docker-vault-injector/internal/environment"
	"github.com/melichron/docker-vault-injector/internal/labels"
	"github.com/melichron/docker-vault-injector/internal/retry"
	statusview "github.com/melichron/docker-vault-injector/internal/status"
	"github.com/melichron/docker-vault-injector/internal/vaultclient"
)

type Docker interface {
	ListServices(ctx context.Context) ([]swarm.Service, error)
	InspectService(ctx context.Context, id string) (swarm.Service, error)
	UpdateService(ctx context.Context, service swarm.Service) ([]string, error)
	WatchServiceEvents(ctx context.Context) (<-chan events.Message, <-chan error)
}

type Vault interface {
	CurrentVersion(ctx context.Context, mount, path string) (int, error)
	ReadVersion(ctx context.Context, mount, path string, version int) (vaultclient.Secret, error)
}

type Controller struct {
	docker             Docker
	vault              Vault
	logger             *slog.Logger
	pollInterval       time.Duration
	reconcileTimeout   time.Duration
	eventRetryInterval time.Duration
	eventRetryMaximum  time.Duration
	status             *statusview.Store
}

func New(
	docker Docker,
	vault Vault,
	logger *slog.Logger,
	pollInterval time.Duration,
	reconcileTimeout time.Duration,
	eventRetryInterval time.Duration,
	eventRetryMaximum time.Duration,
	statusStore *statusview.Store,
) *Controller {
	return &Controller{
		docker:             docker,
		vault:              vault,
		logger:             logger,
		pollInterval:       pollInterval,
		reconcileTimeout:   reconcileTimeout,
		eventRetryInterval: eventRetryInterval,
		eventRetryMaximum:  eventRetryMaximum,
		status:             statusStore,
	}
}

// Run performs an initial full reconciliation, then reacts both to Docker
// service events and to a periodic resync. Events provide low latency; polling
// provides correctness after missed events, daemon restarts, or temporary
// network failures.
func (c *Controller) Run(ctx context.Context) error {
	triggers := make(chan string, 128)
	go c.watchEvents(ctx, triggers)
	c.touchStatus()

	c.reconcileAllWithLogging(ctx, "startup")

	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case serviceID := <-triggers:
			c.touchStatus()
			c.reconcileIDWithLogging(ctx, serviceID, "docker-event")
		case <-ticker.C:
			c.touchStatus()
			c.reconcileAllWithLogging(ctx, "periodic-resync")
		}
	}
}

// watchEvents reconnects forever. It uses a non-blocking send because a burst
// of service updates can safely collapse into the next periodic reconciliation.
func (c *Controller) watchEvents(ctx context.Context, triggers chan<- string) {
	reconnectBackoff := retry.NewBackoff(c.eventRetryInterval, c.eventRetryMaximum)
	for ctx.Err() == nil {
		streamStarted := time.Now()
		receivedEvent := false
		messages, errorsChannel := c.docker.WatchServiceEvents(ctx)
		streamEnded := false

		for !streamEnded {
			select {
			case <-ctx.Done():
				return
			case event, open := <-messages:
				if !open {
					streamEnded = true
					continue
				}
				if event.Actor.ID == "" {
					continue
				}
				receivedEvent = true
				select {
				case triggers <- event.Actor.ID:
				default:
					c.logger.Warn("event queue is full; periodic resync will recover", "service_id", event.Actor.ID)
				}
			case err, open := <-errorsChannel:
				if open && err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
					c.logger.Warn("Docker event stream ended", "error", err)
				}
				streamEnded = true
			}
		}

		if receivedEvent || time.Since(streamStarted) >= c.eventRetryInterval {
			reconnectBackoff.Reset()
		}
		delay := reconnectBackoff.Next()
		c.logger.Debug("reconnecting Docker event stream", "retry_after", delay)
		if !waitForContext(ctx, delay) {
			return
		}
	}
}

func (c *Controller) reconcileAllWithLogging(parent context.Context, reason string) {
	c.touchStatus()
	listContext, cancelList := context.WithTimeout(parent, c.reconcileTimeout)
	services, err := c.docker.ListServices(listContext)
	cancelList()
	if err != nil {
		c.logger.Error("full reconciliation failed", "reason", reason, "error", err)
		return
	}

	c.logger.Debug("starting full reconciliation", "reason", reason, "services", len(services))
	activeServiceIDs := make([]string, 0, len(services))
	for _, service := range services {
		activeServiceIDs = append(activeServiceIDs, service.ID)
		// One slow or unavailable Vault path must not consume the timeout budget
		// of every service that follows it.
		reconcileContext, cancelReconcile := context.WithTimeout(parent, c.reconcileTimeout)
		err := c.ReconcileService(reconcileContext, service)
		cancelReconcile()
		if err != nil {
			c.logger.Error("service reconciliation failed",
				"service", service.Spec.Name,
				"service_id", service.ID,
				"reason", reason,
				"error", err,
			)
		}
		c.touchStatus()
	}
	if c.status != nil {
		if err := c.status.Prune(activeServiceIDs); err != nil {
			c.logger.Warn("cannot prune status snapshot", "error", err)
		}
	}
	c.touchStatus()
}

func (c *Controller) reconcileIDWithLogging(parent context.Context, serviceID, reason string) {
	ctx, cancel := context.WithTimeout(parent, c.reconcileTimeout)
	defer cancel()

	service, err := c.docker.InspectService(ctx, serviceID)
	if err != nil {
		// A service may be removed between receiving its event and inspecting it.
		// Treating this as an ordinary error is harmless; the periodic loop does
		// not retain deleted service IDs.
		c.logger.Debug("cannot inspect service from event", "service_id", serviceID, "error", err)
		return
	}
	if err := c.ReconcileService(ctx, service); err != nil {
		c.logger.Error("service reconciliation failed",
			"service", service.Spec.Name,
			"service_id", service.ID,
			"reason", reason,
			"error", err,
		)
	}
}

// ReconcileService is public within the package API primarily to make the core
// behavior easy to test. It performs at most one Docker ServiceUpdate.
func (c *Controller) ReconcileService(ctx context.Context, service swarm.Service) (reconcileErr error) {
	tracked := service.Spec.Labels[labels.EnabledLabel] != "" ||
		labels.HasControllerState(service.Spec.Labels) ||
		hasBootstrapGate(service)
	if !tracked {
		c.removeStatus(service.ID)
		return nil
	}

	report := statusview.Service{
		ID:    service.ID,
		Name:  service.Spec.Name,
		State: statusview.StateReady,
		Gate:  statusview.GateNotUsed,
	}
	if hasBootstrapGate(service) {
		report.Gate = statusview.GateClosed
	}
	defer func() {
		if reconcileErr != nil {
			report.State = statusview.StateError
			report.Error = reconcileErr.Error()
		}
		c.recordStatus(report, reconcileErr == nil)
	}()

	configuration, err := labels.ParseConfig(service.Spec.Labels)
	if err != nil {
		return err
	}
	if !configuration.Enabled {
		report.State = statusview.StateDisabled
		warnings, updated, err := c.removeManagedEnvironment(ctx, service)
		report.Warnings = warnings
		report.UpdateAttempted = updated
		return err
	}
	report.Sources = sortedSourceNames(configuration.Secrets)
	if configuration.BootstrapGate && report.Gate != statusview.GateClosed {
		report.Gate = statusview.GateOpen
	}
	if service.Spec.TaskTemplate.ContainerSpec == nil {
		return fmt.Errorf("service does not use a container task specification")
	}

	appliedVersions, err := labels.ParseAppliedVersions(service.Spec.Labels)
	if err != nil {
		return err
	}
	report.Versions = appliedVersions
	previouslyManaged, err := labels.ParseManagedEnvironment(service.Spec.Labels)
	if err != nil {
		return err
	}
	report.EnvironmentNames = previouslyManaged
	configurationHash, err := labels.ConfigHash(configuration)
	if err != nil {
		return err
	}
	bootstrapGatePresent := hasBootstrapGate(service)
	if bootstrapGatePresent && !configuration.BootstrapGate {
		return fmt.Errorf("service contains reserved placement constraint %q but %s is not true", labels.BootstrapGateConstraint, labels.BootstrapGateLabel)
	}
	hasSuccessfulInjectionState := service.Spec.Labels[labels.ConfigHashLabel] == configurationHash &&
		service.Spec.Labels[labels.AppliedVersionsLabel] != "" &&
		service.Spec.Labels[labels.ManagedEnvLabel] != "" &&
		service.Spec.Labels[labels.StateHashLabel] != ""
	if configuration.BootstrapGate &&
		!bootstrapGatePresent &&
		!hasSuccessfulInjectionState {
		report.Gate = statusview.GateMissing
		return fmt.Errorf("%s is true but placement constraint %q is missing; refusing an ungated initial or configuration-changing injection", labels.BootstrapGateLabel, labels.BootstrapGateConstraint)
	}

	currentVersions := make(map[string]int, len(configuration.Secrets))
	sourceNames := sortedSourceNames(configuration.Secrets)
	for _, sourceName := range sourceNames {
		source := configuration.Secrets[sourceName]
		version, err := c.vault.CurrentVersion(ctx, source.Mount, source.Path)
		if err != nil {
			return fmt.Errorf("secret source %q: %w", sourceName, err)
		}
		currentVersions[sourceName] = version
	}
	report.Versions = currentVersions

	// In automatic-import mode the desired environment names live in Vault,
	// not in service labels. The names recorded by the previous successful
	// reconciliation are therefore the only names we can use for this cheap
	// drift check without reading the secret data again.
	currentManagedValues := environment.Select(service.Spec.TaskTemplate.ContainerSpec.Env, previouslyManaged)
	stateIsCurrent := maps.Equal(currentVersions, appliedVersions) &&
		service.Spec.Labels[labels.ConfigHashLabel] == configurationHash &&
		environment.Hash(currentManagedValues) == service.Spec.Labels[labels.StateHashLabel] &&
		!bootstrapGatePresent
	if stateIsCurrent {
		return nil
	}

	desiredValues := make(map[string]string)
	environmentOwners := make(map[string]string)
	for _, sourceName := range sourceNames {
		source := configuration.Secrets[sourceName]
		version := currentVersions[sourceName]
		secret, err := c.vault.ReadVersion(ctx, source.Mount, source.Path, version)
		if err != nil {
			return fmt.Errorf("secret source %q: %w", sourceName, err)
		}
		if secret.Version != version {
			return fmt.Errorf("secret source %q: requested version %d but Vault returned version %d", sourceName, version, secret.Version)
		}

		if len(source.Env) == 0 {
			if len(secret.Data) == 0 {
				return fmt.Errorf("secret source %q: automatic import cannot import an empty Vault document", sourceName)
			}
			fieldNames := sortedFieldNames(secret.Data)
			for _, fieldName := range fieldNames {
				if err := validateDockerEnvironmentName(fieldName); err != nil {
					return fmt.Errorf("secret source %q: top-level field %q: %w", sourceName, fieldName, err)
				}
				value, err := stringifyScalar(secret.Data[fieldName], fmt.Sprintf("top-level field %q", fieldName))
				if err != nil {
					return fmt.Errorf("secret source %q: %w", sourceName, err)
				}
				if err := addDesiredEnvironment(desiredValues, environmentOwners, fieldName, value, sourceName); err != nil {
					return err
				}
			}
			continue
		}

		environmentNames := sortedMappingNames(source.Env)
		for _, environmentName := range environmentNames {
			fieldPath := source.Env[environmentName]
			value, err := resolveField(secret.Data, fieldPath)
			if err != nil {
				return fmt.Errorf("secret source %q, environment %s: %w", sourceName, environmentName, err)
			}
			if err := addDesiredEnvironment(desiredValues, environmentOwners, environmentName, value, sourceName); err != nil {
				return err
			}
		}
	}
	desiredNames := sortedMappingNames(desiredValues)
	report.EnvironmentNames = desiredNames

	updated := cloneServiceForUpdate(service)
	updated.Spec.TaskTemplate.ContainerSpec.Env = environment.Merge(
		service.Spec.TaskTemplate.ContainerSpec.Env,
		previouslyManaged,
		desiredValues,
	)

	versionsJSON, managedJSON, err := labels.MarshalState(currentVersions, desiredNames)
	if err != nil {
		return err
	}
	updated.Spec.Labels[labels.AppliedVersionsLabel] = versionsJSON
	updated.Spec.Labels[labels.ManagedEnvLabel] = managedJSON
	updated.Spec.Labels[labels.StateHashLabel] = environment.Hash(desiredValues)
	updated.Spec.Labels[labels.ConfigHashLabel] = configurationHash
	bootstrapGateRemoved := false
	if configuration.BootstrapGate {
		bootstrapGateRemoved = removeBootstrapGate(&updated)
	}

	if slices.Equal(service.Spec.TaskTemplate.ContainerSpec.Env, updated.Spec.TaskTemplate.ContainerSpec.Env) &&
		maps.Equal(service.Spec.Labels, updated.Spec.Labels) &&
		slices.Equal(placementConstraints(service), placementConstraints(updated)) {
		return nil
	}

	warnings, err := c.docker.UpdateService(ctx, updated)
	if err != nil {
		return err
	}
	report.Warnings = warnings
	report.UpdateAttempted = true
	c.logDockerWarnings(service, warnings)
	if bootstrapGateRemoved {
		report.Gate = statusview.GateOpen
	}
	c.logger.Info("applied Vault environment",
		"service", service.Spec.Name,
		"service_id", service.ID,
		"versions", versionsJSON,
		"environment_names", desiredNames,
		"bootstrap_gate_removed", bootstrapGateRemoved,
	)
	return nil
}

func placementConstraints(service swarm.Service) []string {
	if service.Spec.TaskTemplate.Placement == nil {
		return nil
	}
	return service.Spec.TaskTemplate.Placement.Constraints
}

func (c *Controller) recordStatus(service statusview.Service, successful bool) {
	if c.status == nil {
		return
	}
	if err := c.status.Record(service, successful); err != nil {
		c.logger.Warn("cannot write service status", "service", service.Name, "service_id", service.ID, "error", err)
	}
}

func (c *Controller) removeStatus(serviceID string) {
	if c.status == nil {
		return
	}
	if err := c.status.Remove(serviceID); err != nil {
		c.logger.Warn("cannot remove service status", "service_id", serviceID, "error", err)
	}
}

func (c *Controller) touchStatus() {
	if c.status == nil {
		return
	}
	if err := c.status.Heartbeat(); err != nil {
		c.logger.Warn("cannot write controller heartbeat", "error", err)
	}
}

func (c *Controller) logDockerWarnings(service swarm.Service, warnings []string) {
	for _, warning := range warnings {
		c.logger.Warn("Docker accepted the service update with a warning",
			"service", service.Spec.Name,
			"service_id", service.ID,
			"warning", warning,
		)
	}
}

func (c *Controller) removeManagedEnvironment(ctx context.Context, service swarm.Service) ([]string, bool, error) {
	if !labels.HasControllerState(service.Spec.Labels) {
		return nil, false, nil
	}
	if service.Spec.TaskTemplate.ContainerSpec == nil {
		return nil, false, fmt.Errorf("cannot clean controller state from a service without a container task specification")
	}

	managed, err := labels.ParseManagedEnvironment(service.Spec.Labels)
	if err != nil {
		return nil, false, err
	}
	updated := cloneServiceForUpdate(service)
	updated.Spec.TaskTemplate.ContainerSpec.Env = environment.Remove(
		service.Spec.TaskTemplate.ContainerSpec.Env,
		managed,
	)
	delete(updated.Spec.Labels, labels.AppliedVersionsLabel)
	delete(updated.Spec.Labels, labels.ManagedEnvLabel)
	delete(updated.Spec.Labels, labels.StateHashLabel)
	delete(updated.Spec.Labels, labels.ConfigHashLabel)

	warnings, err := c.docker.UpdateService(ctx, updated)
	if err != nil {
		return nil, false, err
	}
	c.logDockerWarnings(service, warnings)
	c.logger.Info("removed Vault-managed environment",
		"service", service.Spec.Name,
		"service_id", service.ID,
		"environment_names", managed,
	)
	return warnings, true, nil
}

// cloneServiceForUpdate copies exactly the mutable map, pointer, and slice that
// this controller edits. Every unrelated ServiceSpec field is preserved.
func cloneServiceForUpdate(service swarm.Service) swarm.Service {
	updated := service
	updated.Spec.Labels = maps.Clone(service.Spec.Labels)
	if updated.Spec.Labels == nil {
		updated.Spec.Labels = make(map[string]string)
	}
	if original := service.Spec.TaskTemplate.ContainerSpec; original != nil {
		containerSpec := *original
		containerSpec.Env = slices.Clone(original.Env)
		updated.Spec.TaskTemplate.ContainerSpec = &containerSpec
	}
	if original := service.Spec.TaskTemplate.Placement; original != nil {
		placement := *original
		placement.Constraints = slices.Clone(original.Constraints)
		updated.Spec.TaskTemplate.Placement = &placement
	}
	return updated
}

func hasBootstrapGate(service swarm.Service) bool {
	placement := service.Spec.TaskTemplate.Placement
	if placement == nil {
		return false
	}
	for _, constraint := range placement.Constraints {
		if isBootstrapGateConstraint(constraint) {
			return true
		}
	}
	return false
}

// removeBootstrapGate removes every copy of the reserved constraint while
// preserving the order and exact spelling of every operator-owned constraint.
// The service must already have been cloned with cloneServiceForUpdate.
func removeBootstrapGate(service *swarm.Service) bool {
	placement := service.Spec.TaskTemplate.Placement
	if placement == nil {
		return false
	}

	constraints := placement.Constraints[:0]
	removed := false
	for _, constraint := range placement.Constraints {
		if isBootstrapGateConstraint(constraint) {
			removed = true
			continue
		}
		constraints = append(constraints, constraint)
	}
	placement.Constraints = constraints
	return removed
}

func isBootstrapGateConstraint(constraint string) bool {
	// Compose accepts whitespace around the equality operator. Compare a
	// whitespace-free representation so the documented forms behave equally.
	return strings.Join(strings.Fields(constraint), "") == labels.BootstrapGateConstraint
}

func resolveField(data map[string]any, fieldPath string) (string, error) {
	parts := strings.Split(fieldPath, ".")
	var current any = data
	for _, part := range parts {
		if part == "" {
			return "", fmt.Errorf("field path %q contains an empty component", fieldPath)
		}
		object, ok := current.(map[string]any)
		if !ok {
			return "", fmt.Errorf("field path %q crosses a non-object value at %q", fieldPath, part)
		}
		current, ok = object[part]
		if !ok {
			return "", fmt.Errorf("field path %q does not exist", fieldPath)
		}
	}

	return stringifyScalar(current, fmt.Sprintf("field path %q", fieldPath))
}

// stringifyScalar deliberately refuses objects, arrays and null. Docker
// environment entries are strings, so silently JSON-encoding a structured
// value would hide a likely configuration mistake.
func stringifyScalar(current any, description string) (string, error) {
	switch value := current.(type) {
	case string:
		return value, nil
	case bool, float32, float64, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, json.Number:
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("encode scalar %s: %w", description, err)
		}
		return string(encoded), nil
	case nil:
		return "", fmt.Errorf("%s resolves to null", description)
	default:
		return "", fmt.Errorf("%s resolves to an object or array; only scalar values can become environment variables", description)
	}
}

func addDesiredEnvironment(values, owners map[string]string, name, value, sourceName string) error {
	if previousSource, duplicate := owners[name]; duplicate {
		return fmt.Errorf("environment variable %q is produced by both secret source %q and %q", name, previousSource, sourceName)
	}
	owners[name] = sourceName
	values[name] = value
	return nil
}

// Docker represents environment entries as NAME=value. We intentionally do
// not impose shell naming conventions here: dashes, dots and other unusual
// names are an operator choice. Empty names and '=' are structurally
// impossible to represent unambiguously and are the only restrictions.
func validateDockerEnvironmentName(name string) error {
	if name == "" {
		return fmt.Errorf("environment variable name cannot be empty")
	}
	if strings.Contains(name, "=") {
		return fmt.Errorf("environment variable name cannot contain '='")
	}
	return nil
}

func sortedFieldNames(fields map[string]any) []string {
	result := make([]string, 0, len(fields))
	for name := range fields {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func sortedMappingNames[T any](mapping map[string]T) []string {
	result := make([]string, 0, len(mapping))
	for name := range mapping {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func sortedSourceNames(sources map[string]labels.SecretSource) []string {
	result := make([]string, 0, len(sources))
	for name := range sources {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func waitForContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
```

### `internal/dockerclient/client.go`

```go
// Package dockerclient adapts the Moby client to the narrow operations needed
// by the reconciliation controller.
package dockerclient

import (
	"context"
	"fmt"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/swarm"
	moby "github.com/moby/moby/client"
)

type Client struct {
	client *moby.Client
}

func NewFromEnvironment() (*Client, error) {
	client, err := moby.New(moby.FromEnv, moby.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create Docker client: %w", err)
	}
	return &Client{client: client}, nil
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) ListServices(ctx context.Context) ([]swarm.Service, error) {
	result, err := c.client.ServiceList(ctx, moby.ServiceListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list Swarm services: %w", err)
	}
	return result.Items, nil
}

func (c *Client) InspectService(ctx context.Context, id string) (swarm.Service, error) {
	result, err := c.client.ServiceInspect(ctx, id, moby.ServiceInspectOptions{})
	if err != nil {
		return swarm.Service{}, fmt.Errorf("inspect Swarm service %s: %w", id, err)
	}
	return result.Service, nil
}

func (c *Client) UpdateService(ctx context.Context, service swarm.Service) ([]string, error) {
	result, err := c.client.ServiceUpdate(ctx, service.ID, moby.ServiceUpdateOptions{
		Version:          service.Version,
		Spec:             service.Spec,
		RegistryAuthFrom: swarm.RegistryAuthFromSpec,
	})
	if err != nil {
		return nil, fmt.Errorf("update Swarm service %s at version %s: %w", service.ID, service.Version.String(), err)
	}
	return result.Warnings, nil
}

// WatchServiceEvents creates one event stream. The controller reconnects when
// this stream ends, because daemon restarts and manager failovers are normal.
func (c *Client) WatchServiceEvents(ctx context.Context) (<-chan events.Message, <-chan error) {
	filters := make(moby.Filters).
		Add("type", string(events.ServiceEventType)).
		Add("event", string(events.ActionCreate), string(events.ActionUpdate))
	result := c.client.Events(ctx, moby.EventsListOptions{Filters: filters})
	return result.Messages, result.Err
}
```

### `internal/environment/environment_test.go`

```go
package environment

import (
	"reflect"
	"testing"
)

func TestMergePreservesUnmanagedAndRemovesOldManagedNames(t *testing.T) {
	input := []string{"LOG_LEVEL=info", "OLD_SECRET=old", "DB_USER=old-user"}
	got := Merge(input, []string{"OLD_SECRET", "DB_USER"}, map[string]string{
		"DB_USER":     "new-user",
		"DB_PASSWORD": "new-password",
	})
	want := []string{"LOG_LEVEL=info", "DB_PASSWORD=new-password", "DB_USER=new-user"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Merge() = %v, want %v", got, want)
	}
}

func TestHashIsIndependentOfMapIterationOrder(t *testing.T) {
	one := map[string]string{"A": "one", "B": "two"}
	two := map[string]string{"B": "two", "A": "one"}
	if Hash(one) != Hash(two) {
		t.Fatal("Hash must be deterministic")
	}
}

func TestSelectExposesMissingVariableAsDrift(t *testing.T) {
	got := Select([]string{"A=one"}, []string{"A", "B"})
	want := map[string]string{"A": "one"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Select() = %v, want %v", got, want)
	}
}
```

### `internal/environment/environment.go`

```go
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
```

### `internal/labels/labels_test.go`

```go
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

func TestParseBootstrapGate(t *testing.T) {
	configuration, err := ParseConfig(map[string]string{
		EnabledLabel:                        "true",
		BootstrapGateLabel:                  "true",
		SecretsPrefix + "common.name":       "common-vars",
		SecretsPrefix + "common.kv":         "kv",
		SecretsPrefix + "common.vault-path": "apps/common",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !configuration.BootstrapGate {
		t.Fatal("bootstrap gate was not enabled")
	}
}

func TestParseBootstrapGateRejectsInvalidBoolean(t *testing.T) {
	_, err := ParseConfig(map[string]string{
		EnabledLabel:                        "true",
		BootstrapGateLabel:                  "sometimes",
		SecretsPrefix + "common.name":       "common-vars",
		SecretsPrefix + "common.kv":         "kv",
		SecretsPrefix + "common.vault-path": "apps/common",
	})
	if err == nil {
		t.Fatal("invalid bootstrap-gate boolean should fail")
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

func TestConfigHashIncludesBootstrapGate(t *testing.T) {
	configuration := Config{
		Enabled: true,
		Secrets: map[string]SecretSource{
			"source": {Name: "source", Mount: "kv", Path: "apps/source"},
		},
	}
	withoutGate, err := ConfigHash(configuration)
	if err != nil {
		t.Fatal(err)
	}
	configuration.BootstrapGate = true
	withGate, err := ConfigHash(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if withoutGate == withGate {
		t.Fatal("bootstrap gate must be part of the configuration hash")
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
```

### `internal/labels/labels.go`

```go
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
	BootstrapGateLabel   = Prefix + ".bootstrap-gate"
	SecretsPrefix        = Prefix + ".secrets."
	AppliedVersionsLabel = Prefix + ".applied-versions"
	ManagedEnvLabel      = Prefix + ".managed-env"
	StateHashLabel       = Prefix + ".state-hash"
	ConfigHashLabel      = Prefix + ".config-hash"

	// BootstrapGateConstraint is an intentionally unsatisfiable placement
	// constraint. Operators must never add the matching label to a Swarm node.
	// When bootstrap-gate is enabled, the controller removes this exact
	// constraint in the same ServiceUpdate that installs the Vault environment.
	BootstrapGateConstraint = "node.labels." + Prefix + ".gate==open"
)

// Config is the user-controlled portion of the service configuration.
type Config struct {
	Enabled       bool
	BootstrapGate bool
	Secrets       map[string]SecretSource
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

	if rawBootstrapGate, exists := serviceLabels[BootstrapGateLabel]; exists {
		bootstrapGate, err := strconv.ParseBool(rawBootstrapGate)
		if err != nil {
			return Config{}, fmt.Errorf("%s must be true or false: %w", BootstrapGateLabel, err)
		}
		result.BootstrapGate = bootstrapGate
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
	// Enabled is deliberately omitted: ConfigHash is only calculated for an
	// enabled service. BootstrapGate is included because turning the safety
	// contract on or off is a material configuration change.
	hashInput := struct {
		BootstrapGate bool                    `json:"bootstrap_gate"`
		Secrets       map[string]SecretSource `json:"secrets"`
	}{
		BootstrapGate: configuration.BootstrapGate,
		Secrets:       configuration.Secrets,
	}
	encoded, err := json.Marshal(hashInput)
	if err != nil {
		return "", fmt.Errorf("marshal injector configuration: %w", err)
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
```

### `internal/retry/backoff_test.go`

```go
package retry

import (
	"testing"
	"time"
)

func TestBackoffGrowsToMaximumAndResets(t *testing.T) {
	backoff := NewBackoff(100*time.Millisecond, 400*time.Millisecond)

	assertWithinJitter(t, backoff.Next(), 100*time.Millisecond)
	assertWithinJitter(t, backoff.Next(), 200*time.Millisecond)
	assertWithinJitter(t, backoff.Next(), 400*time.Millisecond)
	assertWithinJitter(t, backoff.Next(), 400*time.Millisecond)

	backoff.Reset()
	assertWithinJitter(t, backoff.Next(), 100*time.Millisecond)
}

func TestBackoffRaisesMaximumBelowInitial(t *testing.T) {
	backoff := NewBackoff(time.Second, 100*time.Millisecond)
	assertWithinJitter(t, backoff.Next(), time.Second)
	assertWithinJitter(t, backoff.Next(), time.Second)
}

func assertWithinJitter(t *testing.T, actual, base time.Duration) {
	t.Helper()
	minimum := time.Duration(float64(base) * (1 - jitterFraction))
	maximum := time.Duration(float64(base) * (1 + jitterFraction))
	if actual < minimum || actual > maximum {
		t.Fatalf("delay %s is outside [%s, %s] for base %s", actual, minimum, maximum, base)
	}
}
```

### `internal/retry/backoff.go`

```go
// Package retry contains the small amount of retry timing shared by the
// Docker event watcher and Vault authentication. It deliberately does not
// decide which errors are retryable; callers retain that policy.
package retry

import (
	"math/rand/v2"
	"time"
)

const jitterFraction = 0.20

// Backoff grows exponentially from Initial to Max. Next adds up to 20 percent
// positive or negative jitter so several restarted controllers do not retry in
// lockstep. A successful operation should call Reset.
type Backoff struct {
	Initial time.Duration
	Max     time.Duration
	current time.Duration
}

func NewBackoff(initial, maximum time.Duration) *Backoff {
	if maximum < initial {
		maximum = initial
	}
	return &Backoff{Initial: initial, Max: maximum}
}

func (b *Backoff) Next() time.Duration {
	base := b.current
	if base == 0 {
		base = b.Initial
	}

	if base >= b.Max/2 {
		b.current = b.Max
	} else {
		b.current = base * 2
	}

	jitterRange := int64(float64(base) * jitterFraction)
	if jitterRange <= 0 {
		return base
	}
	// Int64N is exclusive at the upper bound, hence +1. The result remains
	// close to the configured maximum while still varying capped retries.
	jitter := rand.Int64N(2*jitterRange+1) - jitterRange
	delay := base + time.Duration(jitter)
	if delay > b.Max {
		return b.Max
	}
	return delay
}

func (b *Backoff) Reset() {
	b.current = 0
}
```

### `internal/status/store_test.go`

```go
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
			Warnings:         []string{"registry auth was not updated"},
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
		"registry auth was not updated",
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
		HeartbeatAt: lastAttempt,
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
			Warnings:         []string{"warning one"},
		}},
	}

	var output bytes.Buffer
	if err := WriteYAML(&output, snapshot); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"generated_at: 2026-08-02T10:01:00Z",
		"heartbeat_at: 2026-08-02T10:01:00Z",
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
		"- warning one",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("YAML does not contain %q:\n%s", expected, output.String())
		}
	}
}

func TestHeartbeatAndHealthCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	if err := store.Heartbeat(); err != nil {
		t.Fatal(err)
	}

	snapshot, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.HeartbeatAt.Equal(now) {
		t.Fatalf("heartbeat = %v, want %v", snapshot.HeartbeatAt, now)
	}
	if err := CheckHealth(snapshot, time.Minute, now.Add(59*time.Second)); err != nil {
		t.Fatalf("fresh heartbeat is unhealthy: %v", err)
	}
	if err := CheckHealth(snapshot, time.Minute, now.Add(61*time.Second)); err == nil {
		t.Fatal("stale heartbeat should be unhealthy")
	}
}

func TestStoreKeepsWarningsUntilNextSuccessfulServiceUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	service := Service{
		ID:              "service-id",
		Name:            "stack_api",
		State:           StateReady,
		Gate:            GateNotUsed,
		Warnings:        []string{"daemon warning"},
		UpdateAttempted: true,
	}
	if err := store.Record(service, true); err != nil {
		t.Fatal(err)
	}
	service.Warnings = nil
	service.UpdateAttempted = false
	if err := store.Record(service, true); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(snapshot.Services[0].Warnings, " "), "daemon warning") {
		t.Fatalf("no-op reconciliation erased warnings: %v", snapshot.Services[0].Warnings)
	}

	service.UpdateAttempted = true
	if err := store.Record(service, true); err != nil {
		t.Fatal(err)
	}
	snapshot, err = Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Services[0].Warnings) != 0 {
		t.Fatalf("clean service update did not clear warnings: %v", snapshot.Services[0].Warnings)
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
```

### `internal/status/store.go`

```go
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
```

### `internal/vaultclient/client_test.go`

```go
package vaultclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	vault "github.com/hashicorp/vault/api"
)

func TestAuthenticateAppRoleUsesConfiguredPathAndCredentialFiles(t *testing.T) {
	credentialDirectory := t.TempDir()
	roleIDFile := filepath.Join(credentialDirectory, "role-id")
	secretIDFile := filepath.Join(credentialDirectory, "secret-id")
	if err := os.WriteFile(roleIDFile, []byte("role-from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretIDFile, []byte("secret-from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/auth/swarm/login" {
			t.Errorf("request path = %q, want /v1/auth/swarm/login", request.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body["role_id"] != "role-from-file" || body["secret_id"] != "secret-from-file" {
			t.Errorf("unexpected login body: %#v", body)
		}
		writeAuthResponse(response, "token-from-approle", 60, true)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.Authenticate(context.Background(), AuthConfig{
		Method:              AuthMethodAppRole,
		AppRoleAuthPath:     "/auth/swarm/",
		AppRoleRoleID:       "ignored-direct-role",
		AppRoleRoleIDFile:   roleIDFile,
		AppRoleSecretID:     "ignored-direct-secret",
		AppRoleSecretIDFile: secretIDFile,
		TokenCheckInterval:  time.Second,
		AuthRetryInterval:   time.Second,
	})
	if err != nil {
		t.Fatalf("Authenticate returned an error: %v", err)
	}
	if got := client.client.Token(); got != "token-from-approle" {
		t.Fatalf("client token = %q", got)
	}
}

func TestTokenLifecycleReauthenticatesWhenLookupDetectsRevocation(t *testing.T) {
	var loginCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/auth/swarm/login":
			login := loginCount.Add(1)
			writeAuthResponse(response, fmt.Sprintf("token-%d", login), 60, true)
		case "/v1/auth/token/lookup-self":
			if loginCount.Load() == 1 {
				response.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(response, `{"errors":["permission denied"]}`)
				return
			}
			writeLookupResponse(response)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	configuration := AuthConfig{
		Method:             AuthMethodAppRole,
		AppRoleAuthPath:    "auth/swarm",
		AppRoleRoleID:      "role",
		AppRoleSecretID:    "secret",
		TokenCheckInterval: 10 * time.Millisecond,
		AuthRetryInterval:  10 * time.Millisecond,
	}
	if err := client.Authenticate(context.Background(), configuration); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		client.RunTokenLifecycle(ctx, discardLogger())
	}()

	waitForCondition(t, 2*time.Second, func() bool { return loginCount.Load() >= 2 })
	if got := client.client.Token(); got != "token-2" {
		t.Fatalf("client did not switch to replacement token: %q", got)
	}
	cancel()
	<-done
}

func TestTokenLifecycleRenewsBeforeTTLExpires(t *testing.T) {
	var renewCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/auth/swarm/login":
			writeAuthResponse(response, "token-1", 1, true)
		case "/v1/auth/token/lookup-self":
			writeLookupResponse(response)
		case "/v1/auth/token/renew-self":
			renewCount.Add(1)
			// Vault renewal responses carry updated auth lease metadata. They do
			// not need to repeat the client token.
			writeAuthResponse(response, "", 2, true)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	if err := client.Authenticate(context.Background(), AuthConfig{
		Method:             AuthMethodAppRole,
		AppRoleAuthPath:    "auth/swarm",
		AppRoleRoleID:      "role",
		AppRoleSecretID:    "secret",
		TokenCheckInterval: 20 * time.Millisecond,
		AuthRetryInterval:  10 * time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		client.RunTokenLifecycle(ctx, discardLogger())
	}()
	waitForCondition(t, 2*time.Second, func() bool { return renewCount.Load() >= 1 })
	cancel()
	<-done
}

func TestNormalizeAppRoleAuthPathRequiresFullMountPath(t *testing.T) {
	for _, invalid := range []string{"", "approle", "auth/", "auth//nested", "auth/approle/login"} {
		if _, err := normalizeAppRoleAuthPath(invalid); err == nil {
			t.Fatalf("normalizeAppRoleAuthPath(%q) should fail", invalid)
		}
	}
	if got, err := normalizeAppRoleAuthPath("/auth/custom/"); err != nil || got != "auth/custom" {
		t.Fatalf("normalized path = %q, err = %v", got, err)
	}
}

func TestTokenStatusFromLookupRejectsExpiredToken(t *testing.T) {
	_, _, err := tokenStatusFromLookup(&vault.Secret{Data: map[string]any{
		"ttl":       json.Number("0"),
		"renewable": true,
	}})
	if err == nil {
		t.Fatal("zero TTL should be treated as an expired token")
	}
}

func newTestClient(t *testing.T, address string) *Client {
	t.Helper()
	configuration := vault.DefaultConfig()
	configuration.Address = address
	configuration.MaxRetries = 0
	raw, err := vault.NewClient(configuration)
	if err != nil {
		t.Fatal(err)
	}
	return &Client{client: raw}
}

func writeAuthResponse(response http.ResponseWriter, token string, ttl int, renewable bool) {
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(map[string]any{
		"auth": map[string]any{
			"client_token":   token,
			"lease_duration": ttl,
			"renewable":      renewable,
		},
	})
}

func writeLookupResponse(response http.ResponseWriter) {
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(map[string]any{
		"data": map[string]any{"ttl": 60, "renewable": true},
	})
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
```

### `internal/vaultclient/client.go`

```go
// Package vaultclient is a deliberately thin adapter around HashiCorp's
// official Vault client. Besides KV v2 access it owns authentication because
// the token and the client using that token must have one lifecycle.
package vaultclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	vault "github.com/hashicorp/vault/api"

	"github.com/melichron/docker-vault-injector/internal/retry"
)

const (
	AuthMethodAppRole = "approle"
	AuthMethodToken   = "token"
)

type Secret struct {
	Version int
	Data    map[string]any
}

// AuthConfig is copied from process configuration so this package remains
// independent from internal/config. File values take precedence over direct
// environment values and are re-read before every AppRole login.
type AuthConfig struct {
	Method              string
	AppRoleAuthPath     string
	AppRoleRoleID       string
	AppRoleRoleIDFile   string
	AppRoleSecretID     string
	AppRoleSecretIDFile string
	Token               string
	TokenFile           string
	TokenCheckInterval  time.Duration
	AuthRetryInterval   time.Duration
	AuthRetryMaximum    time.Duration
}

type tokenLease struct {
	renewable bool
	renewAt   time.Time
}

type Client struct {
	client *vault.Client
	auth   AuthConfig
	lease  tokenLease
}

// NewFromEnvironment honors the standard VAULT_ADDR, VAULT_CACERT,
// VAULT_CAPATH, VAULT_CLIENT_CERT, VAULT_CLIENT_KEY, VAULT_NAMESPACE and proxy
// environment variables understood by the official client.
func NewFromEnvironment() (*Client, error) {
	configuration := vault.DefaultConfig()
	if err := configuration.ReadEnvironment(); err != nil {
		return nil, fmt.Errorf("read Vault environment: %w", err)
	}

	client, err := vault.NewClient(configuration)
	if err != nil {
		return nil, fmt.Errorf("create Vault client: %w", err)
	}
	return &Client{client: client}, nil
}

// Authenticate validates configuration and obtains the initial token before
// the reconciliation controller starts. This avoids starting in a state where
// every service immediately fails with an unauthenticated Vault request.
func (c *Client) Authenticate(ctx context.Context, configuration AuthConfig) error {
	configuration.Method = strings.ToLower(strings.TrimSpace(configuration.Method))
	if configuration.Method == "" {
		configuration.Method = AuthMethodAppRole
	}
	c.auth = configuration

	switch configuration.Method {
	case AuthMethodAppRole:
		path, err := normalizeAppRoleAuthPath(configuration.AppRoleAuthPath)
		if err != nil {
			return err
		}
		c.auth.AppRoleAuthPath = path
		if configuration.TokenCheckInterval <= 0 {
			return fmt.Errorf("Vault token check interval must be greater than zero")
		}
		if configuration.AuthRetryInterval <= 0 {
			return fmt.Errorf("Vault auth retry interval must be greater than zero")
		}
		if configuration.AuthRetryMaximum == 0 {
			configuration.AuthRetryMaximum = configuration.AuthRetryInterval
			c.auth.AuthRetryMaximum = configuration.AuthRetryMaximum
		}
		if configuration.AuthRetryMaximum < configuration.AuthRetryInterval {
			return fmt.Errorf("Vault auth retry maximum must be greater than or equal to the initial interval")
		}

		secret, err := c.loginAppRole(ctx)
		if err != nil {
			return err
		}
		lease, err := leaseFromAuthSecret(secret, time.Now())
		if err != nil {
			return err
		}
		c.client.SetToken(secret.Auth.ClientToken)
		c.lease = lease
		return nil

	case AuthMethodToken:
		token, err := readCredential("Vault token", configuration.Token, configuration.TokenFile)
		if err != nil {
			return err
		}
		c.client.SetToken(token)
		if _, err := c.client.Auth().Token().LookupSelfWithContext(ctx); err != nil {
			return fmt.Errorf("validate static Vault token: %w", err)
		}
		return nil

	default:
		return fmt.Errorf("unsupported Vault auth method %q", configuration.Method)
	}
}

// MaintainsToken reports whether this client needs the background lifecycle.
// Static token mode is intentionally static and exists only as an explicit
// development/fallback option.
func (c *Client) MaintainsToken() bool {
	return c.auth.Method == AuthMethodAppRole
}

// RunTokenLifecycle periodically proves that the AppRole token is still valid,
// renews it before its lease expires, and obtains a fresh token whenever lookup
// or renewal fails. It should run as one goroutine for the lifetime of Client.
//
// RoleID and SecretID files are re-read by every login. Operators can therefore
// rotate either credential atomically without restarting the controller.
func (c *Client) RunTokenLifecycle(ctx context.Context, logger *slog.Logger) {
	if !c.MaintainsToken() {
		return
	}

	for ctx.Err() == nil {
		wait := c.auth.TokenCheckInterval
		untilRenewal := time.Until(c.lease.renewAt)
		if untilRenewal < wait {
			wait = untilRenewal
		}
		if wait < 0 {
			wait = 0
		}
		if !waitForContext(ctx, wait) {
			return
		}

		// lookup-self detects externally revoked tokens rather than waiting for
		// the locally calculated TTL to elapse.
		lookup, err := c.client.Auth().Token().LookupSelfWithContext(ctx)
		if err != nil {
			logger.Warn("Vault token is no longer valid; re-authenticating with AppRole", "error", err)
			if !c.reauthenticateUntilSuccess(ctx, logger) {
				return
			}
			continue
		}
		remainingTTL, renewable, err := tokenStatusFromLookup(lookup)
		if err != nil {
			logger.Warn("Vault token lookup returned invalid status; re-authenticating with AppRole", "error", err)
			if !c.reauthenticateUntilSuccess(ctx, logger) {
				return
			}
			continue
		}
		c.lease.renewable = renewable
		// Vault is authoritative. If lookup-self reports less remaining time
		// than our local lease calculation, move renewal earlier.
		lookupRenewAt := time.Now().Add(remainingTTL * 2 / 3)
		if lookupRenewAt.Before(c.lease.renewAt) {
			c.lease.renewAt = lookupRenewAt
		}

		if time.Now().Before(c.lease.renewAt) {
			continue
		}
		if !c.lease.renewable {
			logger.Info("Vault token is not renewable; obtaining a replacement with AppRole")
			if !c.reauthenticateUntilSuccess(ctx, logger) {
				return
			}
			continue
		}

		renewed, err := c.client.Auth().Token().RenewSelfWithContext(ctx, 0)
		if err == nil {
			var lease tokenLease
			lease, err = leaseFromRenewal(renewed, time.Now())
			if err == nil {
				c.lease = lease
				logger.Debug("renewed Vault AppRole token", "next_renewal", lease.renewAt)
				continue
			}
		}

		logger.Warn("cannot renew Vault token; re-authenticating with AppRole", "error", err)
		if !c.reauthenticateUntilSuccess(ctx, logger) {
			return
		}
	}
}

func (c *Client) reauthenticateUntilSuccess(ctx context.Context, logger *slog.Logger) bool {
	backoff := retry.NewBackoff(c.auth.AuthRetryInterval, c.auth.AuthRetryMaximum)
	for ctx.Err() == nil {
		secret, err := c.loginAppRole(ctx)
		if err == nil {
			var lease tokenLease
			lease, err = leaseFromAuthSecret(secret, time.Now())
			if err == nil {
				c.client.SetToken(secret.Auth.ClientToken)
				c.lease = lease
				logger.Info("authenticated to Vault with AppRole",
					"auth_path", c.auth.AppRoleAuthPath,
					"renewable", lease.renewable,
					"next_renewal", lease.renewAt,
				)
				return true
			}
		}

		delay := backoff.Next()
		logger.Error("AppRole authentication failed; will retry",
			"auth_path", c.auth.AppRoleAuthPath,
			"retry_after", delay,
			"error", err,
		)
		if !waitForContext(ctx, delay) {
			return false
		}
	}
	return false
}

func (c *Client) loginAppRole(ctx context.Context) (*vault.Secret, error) {
	roleID, err := readCredential("AppRole RoleID", c.auth.AppRoleRoleID, c.auth.AppRoleRoleIDFile)
	if err != nil {
		return nil, err
	}
	secretID, err := readCredential("AppRole SecretID", c.auth.AppRoleSecretID, c.auth.AppRoleSecretIDFile)
	if err != nil {
		return nil, err
	}

	secret, err := c.client.Logical().WriteWithContext(ctx, c.auth.AppRoleAuthPath+"/login", map[string]any{
		"role_id":   roleID,
		"secret_id": secretID,
	})
	if err != nil {
		return nil, fmt.Errorf("AppRole login at %s: %w", c.auth.AppRoleAuthPath, err)
	}
	if secret == nil || secret.Auth == nil || secret.Auth.ClientToken == "" {
		return nil, fmt.Errorf("AppRole login at %s returned no client token", c.auth.AppRoleAuthPath)
	}
	return secret, nil
}

func normalizeAppRoleAuthPath(value string) (string, error) {
	path := strings.Trim(strings.TrimSpace(value), "/")
	if path == "" {
		return "", fmt.Errorf("VAULT_APPROLE_AUTH_PATH is required for AppRole authentication")
	}
	if !strings.HasPrefix(path, "auth/") {
		return "", fmt.Errorf("VAULT_APPROLE_AUTH_PATH must be a full mount path such as auth/approle")
	}
	if strings.TrimPrefix(path, "auth/") == "" || strings.Contains(path, "//") {
		return "", fmt.Errorf("VAULT_APPROLE_AUTH_PATH must contain a non-empty auth mount name")
	}
	if strings.HasSuffix(path, "/login") {
		return "", fmt.Errorf("VAULT_APPROLE_AUTH_PATH must name the auth mount without /login")
	}
	return path, nil
}

func readCredential(name, directValue, filePath string) (string, error) {
	value := directValue
	if filePath != "" {
		contents, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("read %s file %q: %w", name, filePath, err)
		}
		value = string(contents)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is empty", name)
	}
	return value, nil
}

func leaseFromAuthSecret(secret *vault.Secret, now time.Time) (tokenLease, error) {
	if secret == nil || secret.Auth == nil || secret.Auth.ClientToken == "" {
		return tokenLease{}, fmt.Errorf("Vault authentication response has no client token")
	}
	return leaseFromTTL(secret.Auth.LeaseDuration, secret.Auth.Renewable, now)
}

func leaseFromRenewal(secret *vault.Secret, now time.Time) (tokenLease, error) {
	if secret == nil || secret.Auth == nil {
		return tokenLease{}, fmt.Errorf("Vault token renewal response has no auth metadata")
	}
	return leaseFromTTL(secret.Auth.LeaseDuration, secret.Auth.Renewable, now)
}

func leaseFromTTL(ttlSeconds int, renewable bool, now time.Time) (tokenLease, error) {
	if ttlSeconds <= 0 {
		return tokenLease{}, fmt.Errorf("Vault token has invalid TTL %d", ttlSeconds)
	}
	ttl := time.Duration(ttlSeconds) * time.Second
	return tokenLease{
		renewable: renewable,
		// Renew after two thirds of the granted TTL. This leaves one third for
		// retries or AppRole re-login before the current token actually expires.
		renewAt: now.Add(ttl * 2 / 3),
	}, nil
}

func tokenStatusFromLookup(secret *vault.Secret) (time.Duration, bool, error) {
	if secret == nil || secret.Data == nil {
		return 0, false, fmt.Errorf("Vault token lookup response has no data")
	}

	rawTTL, exists := secret.Data["ttl"]
	if !exists {
		return 0, false, fmt.Errorf("Vault token lookup response has no TTL")
	}
	var ttlSeconds int64
	switch value := rawTTL.(type) {
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0, false, fmt.Errorf("parse Vault token TTL: %w", err)
		}
		ttlSeconds = parsed
	case float64:
		ttlSeconds = int64(value)
	case int:
		ttlSeconds = int64(value)
	case int64:
		ttlSeconds = value
	default:
		return 0, false, fmt.Errorf("Vault token lookup returned unsupported TTL type %T", rawTTL)
	}
	if ttlSeconds <= 0 {
		return 0, false, fmt.Errorf("Vault token is expired or has invalid TTL %d", ttlSeconds)
	}

	renewable, exists := secret.Data["renewable"].(bool)
	if !exists {
		return 0, false, fmt.Errorf("Vault token lookup response has no renewable flag")
	}
	return time.Duration(ttlSeconds) * time.Second, renewable, nil
}

func waitForContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (c *Client) CurrentVersion(ctx context.Context, mount, path string) (int, error) {
	metadata, err := c.client.KVv2(mount).GetMetadata(ctx, path)
	if err != nil {
		return 0, fmt.Errorf("read metadata for %s/%s: %w", mount, path, err)
	}
	if metadata.CurrentVersion <= 0 {
		return 0, fmt.Errorf("Vault returned invalid current version %d for %s/%s", metadata.CurrentVersion, mount, path)
	}
	return metadata.CurrentVersion, nil
}

func (c *Client) ReadVersion(ctx context.Context, mount, path string, version int) (Secret, error) {
	secret, err := c.client.KVv2(mount).GetVersion(ctx, path, version)
	if err != nil {
		return Secret{}, fmt.Errorf("read version %d of %s/%s: %w", version, mount, path, err)
	}
	if secret.Data == nil {
		return Secret{}, fmt.Errorf("version %d of %s/%s is deleted or contains no data", version, mount, path)
	}
	if secret.VersionMetadata == nil {
		return Secret{}, fmt.Errorf("version %d of %s/%s has no version metadata", version, mount, path)
	}
	return Secret{Version: secret.VersionMetadata.Version, Data: secret.Data}, nil
}
```

### `Makefile`

```make
.PHONY: build test vet fmt docker-build

build:
	go build -o bin/docker-vault-injector ./cmd/docker-vault-injector

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(find cmd internal -name '*.go' -type f)

docker-build:
	docker build -t docker-vault-injector:dev .

```

### `README.md`

````markdown
# docker-vault-injector

A small reconciliation controller for Docker Swarm. It reads configuration from service labels, retrieves values from HashiCorp Vault KV v2, and adds them to `ServiceSpec.TaskTemplate.ContainerSpec.Env`. When a secret changes, the controller updates the service and Swarm performs a regular rolling update of its tasks.

The project supports both multi-node clusters and single-node Swarm installations. A single-node Swarm can be initialized with one command:

```bash
docker swarm init
```

## Features

The controller currently provides:

- a Docker service event stream for reacting quickly to service creation and updates;
- periodic full reconciliation in case events are missed;
- Vault KV v2 metadata reads followed by reads of an exact secret version;
- AppRole login through an explicitly configured auth mount path;
- periodic token validation through `lookup-self`;
- `renew-self` before token expiration and a new AppRole login after revocation or expiry;
- multiple Vault documents per service;
- automatic import of flat Vault documents without listing individual variables;
- optional explicit mappings, including nested fields such as `connection.host`;
- environment variable collision detection across sources;
- an opt-in bootstrap gate that prevents tasks from starting before their first successful injection;
- optimistic locking through Docker `Version.Index`;
- preservation of all `ServiceSpec` fields not owned by the controller;
- detection of manual changes to injected environment variables;
- removal of obsolete managed variables when mappings change;
- removal of injected variables when `enabled: "false"`;
- local `status` and `status-yaml` commands showing the last reconciliation result for every managed service;
- a local `health` command that detects a stalled reconciliation loop without treating Vault outages as process failures;
- exponential retry backoff with jitter for initial AppRole authentication, re-authentication, and Docker event reconnection;
- Docker ServiceUpdate warnings in logs and status output;
- no secret values in controller logs;
- unit tests for the main scenarios.

Response-wrapped SecretID delivery, leader election for multiple controller replicas, Prometheus metrics, and full end-to-end tests are not implemented yet.

## How it works

```text
Docker service create/update event        Periodic timer
                 │                              │
                 └──────────┬───────────────────┘
                            ▼
                   inspect Swarm Service
                            │
                    read deploy.labels
                            │
               read current_version from Vault
                            │
          versions, config hash, and state hash match?
                     ┌──────┴──────┐
                    yes            no
                     │              │
                  no-op       read exact versions
                                    │
                              construct Env
                                    │
                             ServiceUpdate
                                    │
                            Swarm rolling update
```

The controller first reads `current_version` from Vault metadata. If an update is required, it requests that exact version through `GetVersion`. This prevents a race where a new Vault version appears between the metadata and data requests.

In addition to versions, the controller stores SHA-256 hashes of the source configuration and the managed portion of the environment. The configuration hash detects label changes even when Vault version numbers happen to match. The state hash detects manual changes to values such as `DB_PASSWORD` in Docker `ServiceSpec`. These hashes are not a security boundary: a user who can read service labels can generally read `ServiceSpec.Env` as well.

## Service labels

Labels must be placed under `deploy.labels`. A regular `labels` block creates container labels, while this controller watches Swarm Services.

The common case is a single flat Vault document. No multiline JSON label is needed:

```yaml
services:
  api:
    image: registry.example.com/api:1.0.0
    deploy:
      labels:
        io.github.docker-vault-injector.enabled: "true"
        io.github.docker-vault-injector.secrets.common.name: "common-vars"
        io.github.docker-vault-injector.secrets.common.kv: "docker-swarm-secrets"
        io.github.docker-vault-injector.secrets.common.vault-path: "stage/backend/common"
```

If that path contains:

```json
{
  "FIRST_ENV": "value",
  "SECOND_ENV": "another_value",
  "PORT": 8080
}
```

the controller adds `FIRST_ENV=value`, `SECOND_ENV=another_value`, and `PORT=8080`. When a source has no `.env.*` labels, every top-level scalar field is imported under its original name.

Multiple sources are expressed as multiple label groups:

```yaml
deploy:
  labels:
    io.github.docker-vault-injector.enabled: "true"
    io.github.docker-vault-injector.secrets.common.name: "common-vars"
    io.github.docker-vault-injector.secrets.common.kv: "docker-swarm-secrets"
    io.github.docker-vault-injector.secrets.common.vault-path: "stage/backend/common"
    io.github.docker-vault-injector.secrets.database.name: "db-vars"
    io.github.docker-vault-injector.secrets.database.kv: "docker-db-secrets"
    io.github.docker-vault-injector.secrets.database.vault-path: "stage/backend/db-conns"
```

`common` and `database` are arbitrary unique group identifiers. `name` is a required, human-readable name that must be unique across sources. `kv` is the name of the KV v2 mount point without `/data/` or `/metadata/`; `vault-path` is the logical path within that mount.

Add an explicit mapping when you need selective imports, renaming, or access to a nested field. The presence of at least one `.env.*` label switches that source from automatic import to selective import:

```yaml
deploy:
  labels:
    io.github.docker-vault-injector.enabled: "true"
    io.github.docker-vault-injector.secrets.database.name: "db-vars"
    io.github.docker-vault-injector.secrets.database.kv: "kv"
    io.github.docker-vault-injector.secrets.database.vault-path: "apps/api/database"
    io.github.docker-vault-injector.secrets.database.env.DB_USER: "username"
    io.github.docker-vault-injector.secrets.database.env.DB_HOST: "connection.host"
```

The suffix after `.env.` is the target Docker environment variable name, while the label value is the path to a field in the Vault document. The controller does not impose shell naming conventions; operators are responsible for deciding whether unusual names are appropriate. Only empty names and names containing `=` are rejected because Docker stores entries as `NAME=value`.

An environment variable may be owned by only one source, including variables discovered through automatic import. A collision rejects the entire reconciliation before the service is changed. Environment values may be strings, numbers, or booleans. Automatic import rejects top-level objects, arrays, and `null`; a nested scalar can instead be selected with an explicit mapping.

## Bootstrap gate

Docker Swarm has no admission hook that lets an event-driven controller mutate a service before the scheduler sees it. A service created normally may therefore start one or more tasks before the injector reacts to its create event.

Initialization-sensitive services can opt into a fail-closed scheduling gate:

```yaml
deploy:
  placement:
    constraints:
      # Ordinary constraints remain untouched.
      - node.role==worker
      - node.labels.region==eu-central

      # Reserved injector gate. Never add the matching label to a node.
      - node.labels.io.github.docker-vault-injector.gate==open
  labels:
    io.github.docker-vault-injector.enabled: "true"
    io.github.docker-vault-injector.bootstrap-gate: "true"
    io.github.docker-vault-injector.secrets.application.name: "application"
    io.github.docker-vault-injector.secrets.application.kv: "kv"
    io.github.docker-vault-injector.secrets.application.vault-path: "apps/application"
```

No Swarm node should ever have this label:

```text
io.github.docker-vault-injector.gate=open
```

The unsatisfied equality constraint keeps tasks in `Pending`. After every Vault source has been read and validated, the controller writes the environment and removes only the reserved constraint in the same optimistic `ServiceUpdate`. The scheduler can then start tasks from the injected service revision. Operator-owned constraints and replica settings are preserved.

If Vault is unavailable, a field is invalid, or two sources collide, the controller performs no update and the gate remains closed. Enabling `bootstrap-gate` without placing the reserved constraint in the initial or configuration-changing service specification is an error; the label by itself cannot stop the scheduler.

Keep both the label and reserved constraint in the stack file for as long as the service depends on injection. The live ServiceSpec intentionally has the gate removed, while a later `docker stack deploy` reapplies a specification that restores the gate and omits the controller-generated environment. The injector then performs another gated reconciliation. Removing the gate from the stack reintroduces the create/update race.

See [examples/postgres-stack.yaml](examples/postgres-stack.yaml) for a complete initialization-sensitive example.

The controller adds these state labels itself:

```text
io.github.docker-vault-injector.applied-versions
io.github.docker-vault-injector.managed-env
io.github.docker-vault-injector.state-hash
io.github.docker-vault-injector.config-hash
```

Do not edit them manually. They do not contain secret values.

## Disabling injection

To remove variables previously injected by the controller, set:

```yaml
deploy:
  labels:
    io.github.docker-vault-injector.enabled: "false"
```

On the next reconciliation, the controller removes the variables listed in `managed-env`, clears its state labels, and updates the service. After cleanup has completed, all injector labels may be removed.

## Vault requirements

Only KV v2 is supported. A minimal policy is provided in [examples/vault-policy.hcl](examples/vault-policy.hcl):

```hcl
path "kv/metadata/apps/*" {
  capabilities = ["read"]
}

path "kv/data/apps/*" {
  capabilities = ["read"]
}

path "auth/token/lookup-self" {
  capabilities = ["read"]
}

path "auth/token/renew-self" {
  capabilities = ["update"]
}
```

A complete example for creating a custom auth mount, policy, AppRole, RoleID, and SecretID is available in [examples/setup-approle.md](examples/setup-approle.md).

Example data:

```bash
vault kv put -mount=kv apps/example/database \
  DB_USER=example \
  DB_PASSWORD=change-me \
  DB_HOST=db.internal \
  DB_PORT=5432
```

Explicit `.env.*` mappings can also select fields from nested JSON:

```bash
vault kv put -mount=kv apps/example/database @database.json
```

```json
{
  "username": "example",
  "password": "change-me",
  "connection": {
    "host": "db.internal",
    "port": 5432
  }
}
```

## Process configuration

The standard environment variables supported by the official Vault Go client are available, including `VAULT_ADDR`, `VAULT_CACERT`, `VAULT_CAPATH`, `VAULT_CLIENT_CERT`, `VAULT_CLIENT_KEY`, `VAULT_NAMESPACE`, and proxy variables.

| Variable | Default | Purpose |
|---|---:|---|
| `VAULT_AUTH_METHOD` | `approle` | `approle`, or the explicit `token` fallback |
| `VAULT_APPROLE_AUTH_PATH` | — | Required full mount path, for example `auth/docker-swarm`, without `/login` |
| `VAULT_APPROLE_ROLE_ID` | — | RoleID supplied directly through the environment |
| `VAULT_APPROLE_ROLE_ID_FILE` | — | File containing RoleID; takes precedence over the environment |
| `VAULT_APPROLE_SECRET_ID` | — | SecretID supplied directly through the environment |
| `VAULT_APPROLE_SECRET_ID_FILE` | — | File containing SecretID; takes precedence over the environment |
| `VAULT_TOKEN_CHECK_INTERVAL` | `30s` | Interval between AppRole token checks through `lookup-self` |
| `VAULT_AUTH_RETRY_INTERVAL` | `5s` | Initial delay after a failed AppRole login |
| `VAULT_AUTH_RETRY_MAX_INTERVAL` | `1m` | Maximum AppRole retry delay, including jitter |
| `VAULT_TOKEN` | — | Static token used when `VAULT_AUTH_METHOD=token` |
| `VAULT_TOKEN_FILE` | — | File containing a static token; takes precedence over `VAULT_TOKEN` |
| `INJECTOR_POLL_INTERVAL` | `30s` | Full reconciliation interval |
| `INJECTOR_RECONCILE_TIMEOUT` | `20s` | Timeout for one Docker/Vault reconciliation |
| `INJECTOR_EVENT_RETRY_INTERVAL` | `5s` | Initial delay before reconnecting to Docker events |
| `INJECTOR_EVENT_RETRY_MAX_INTERVAL` | `1m` | Maximum Docker event reconnect delay, including jitter |
| `INJECTOR_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `INJECTOR_STATUS_FILE` | `/tmp/docker-vault-injector-status.json` | Local snapshot read by the `status` subcommand |
| `INJECTOR_HEALTH_MAX_AGE` | `1m20s` | Maximum accepted controller heartbeat age for `health` |

In AppRole mode, the initial login completes before reconciliation starts. A failed initial login is retried indefinitely with exponential backoff and jitter rather than terminating the process. The injector then checks the token on schedule, renews it after roughly two-thirds of its issued TTL, and performs a new AppRole login if the token is revoked, expired, non-renewable, or cannot be renewed. RoleID and SecretID files are read again on every login.

The `token` mode remains available for development and emergency diagnostics. It validates the static token at startup but intentionally does not manage its lifecycle.

## Building and testing

Go 1.26 or newer is required.

```bash
make test
make vet
make build
```

To build the container image:

```bash
make docker-build
```

## Running in Swarm

First create an AppRole and two Docker Secrets using [examples/setup-approle.md](examples/setup-approle.md):

```bash
docker secret create vault_approle_role_id role-id.txt
docker secret create vault_approle_secret_id secret-id.txt
```

Build the image and deploy the example:

```bash
docker build -t docker-vault-injector:dev .
docker stack deploy -c examples/stack.yaml vault-injector-demo
```

The injector must run on a manager node because the Service API is a manager API. The example enforces this with a placement constraint:

```yaml
deploy:
  replicas: 1
  placement:
    constraints:
      - node.role==manager
```

Inspect the result:

```bash
docker service inspect vault-injector-demo_example \
  --format '{{json .Spec.TaskTemplate.ContainerSpec.Env}}'
```

After writing a new version:

```bash
vault kv patch -mount=kv apps/example/database DB_PASSWORD=new-password
```

the controller notices the new `current_version`, changes the environment, and Swarm replaces the service task.

The Postgres bootstrap-gate example expects a flat Vault document:

```bash
vault kv put -mount=kv apps/example/postgres \
  POSTGRES_USER=example \
  POSTGRES_PASSWORD=change-me \
  POSTGRES_DB=example
```

Deploy it with:

```bash
docker stack deploy -c examples/postgres-stack.yaml vault-postgres-demo
```

While the service is waiting for injection, `docker service ps vault-postgres-demo_postgres` reports that no node satisfies the reserved constraint. After successful injection, the gate disappears from the live service, Postgres starts with all three initialization variables, and its named volume retains the initialized database.

## Status command

The running controller writes a small atomic status snapshot inside its own container. Execute the same binary as a second process to render that snapshot as a table; the command does not reconnect to Docker or Vault:

```bash
docker exec -it INJECTOR_CONTAINER docker-vault-injector status
```

Swarm task container names are dynamic. One convenient way to locate the current injector task is:

```bash
injector_container="$(docker ps \
  --filter label=com.docker.swarm.service.name=vault-injector-demo_injector \
  --format '{{.ID}}' \
  | head -n1)"

docker exec -it "$injector_container" docker-vault-injector status
```

Example output:

```text
SERVICE                      STATE  GATE    SOURCES   ENVIRONMENT                          VAULT VERSIONS  LAST SUCCESS              ERROR                                                WARNINGS
vault-injector-demo_api      ready  -       database  DB_HOST,DB_PASSWORD,DB_USER          database=7     2026-08-02T14:10:00+03:00  -                                                    -
vault-postgres-demo_postgres error  closed  postgres  POSTGRES_DB,POSTGRES_PASSWORD,...    postgres=3     2026-08-02T14:08:12+03:00  secret source "postgres": Vault unavailable          -
```

The snapshot includes service name and ID, state, bootstrap-gate state, source identifiers, environment variable names, Vault versions, timestamps, the last safe reconciliation error, and Docker update warnings. It never contains environment values, Vault data, RoleID, SecretID, or client tokens.

For the complete machine-readable snapshot, including service IDs and all timestamps, use:

```bash
docker exec -i INJECTOR_CONTAINER docker-vault-injector status-yaml
```

Example output:

```yaml
generated_at: 2026-08-02T11:10:03Z
heartbeat_at: 2026-08-02T11:10:03Z
services:
  - id: n4w5b7...
    name: vault-injector-demo_api
    state: ready
    gate: '-'
    sources:
      - database
    environment_names:
      - DB_HOST
      - DB_PASSWORD
      - DB_USER
    versions:
      database: 7
    last_attempt: 2026-08-02T11:10:03Z
    last_success: 2026-08-02T11:10:03Z
    error: ""
    warnings: []
```

`status-yaml` writes only YAML to stdout, so its output can be redirected or piped into tools such as `yq`. A TTY is not required, hence the example uses `docker exec -i` instead of `-it`.

The same heartbeat supports a local liveness check:

```bash
docker exec -i INJECTOR_CONTAINER docker-vault-injector health
```

It prints `healthy` and exits with status zero while the controller loop is making progress. A missing or stale heartbeat produces a non-zero exit status. Individual reconciliation errors and Vault outages do not make the process unhealthy, preventing dependency failures from causing a restart storm. Both example stacks use this command as the injector task healthcheck.

Set `INJECTOR_HEALTH_MAX_AGE` higher than custom polling, reconciliation, or authentication retry intervals. The default `1m20s` covers the default timings.

Only services carrying injector configuration or controller state are shown. Removed services disappear after the next successful full Docker service listing. The snapshot is local to the current injector task and is recreated after that task restarts.

`INJECTOR_STATUS_FILE` normally does not need to be changed. If the container uses a read-only root filesystem, point it at a small writable mount. Failure to write the status file disables status reporting but never stops reconciliation.

## Error behavior

The project's fail-safe rule is: **never replace working values with empty values because of a Vault or configuration error**.

If Vault is unavailable, a version has been deleted, a field is missing, or a mapping is invalid, `ServiceSpec` remains unchanged and the error is logged without secret values. The next Docker event or periodic reconciliation retries the operation.

If another process changes the service between `ServiceInspect` and `ServiceUpdate`, Docker rejects the stale `Version.Index`. The controller does not overwrite newer state; the next event or reconciliation starts again from a fresh inspection.

Initial Vault authentication, later AppRole re-authentication, and Docker event-stream reconnection use exponential backoff with up to 20 percent jitter. Successful long-lived operation resets the corresponding backoff. Docker warnings returned after successful service updates are logged and retained in status until the next successful service update.

## Interaction with `docker stack deploy`

Running `docker stack deploy` again may remove environment variables and state labels added by the controller because they are not present in the original stack YAML. The injector restores them after the service update event. This may cause a second rolling update immediately after the rollout initiated by `docker stack deploy`.

Without a bootstrap gate, this is eventual consistency: a new task may start before reinjection. With a correctly configured bootstrap gate, uninjected task revisions remain unschedulable and the injector removes the gate only together with the restored environment. Depending on the service's `update_config.order`, this safety may trade availability for correctness while Vault is unavailable. Guaranteeing exactly one rollout would require a separate deployment wrapper or preprocessor, which is outside this project's scope.

## Security

The container has access to `/var/run/docker.sock`, which effectively grants administrative control over the Docker host and Swarm. Run only trusted images, and never expose the Docker API externally without TLS.

Secrets are stored in `ServiceSpec.Env` and are visible through `docker service inspect` to users who can manage the Swarm. This is an intentional part of the threat model: the same user can execute a command inside a task container and read its environment.

The controller never writes secret values to its own logs. Do not place secrets in labels or use secret values as part of Vault path names.

## Project structure

```text
cmd/docker-vault-injector/   wiring, signals, process lifecycle
internal/config/             process environment configuration
internal/controller/         reconciliation and event loop
internal/dockerclient/       thin Moby client adapter
internal/environment/        merge, cleanup and drift hash
internal/labels/             public label schema and validation
internal/retry/              exponential retry timing with jitter
internal/status/             secret-free local snapshot, table and YAML rendering
internal/vaultclient/        thin official Vault client adapter
examples/                    Swarm stack and Vault policy
```

Detailed conventions for future developers and coding agents are documented in [AGENTS.md](AGENTS.md).
````

