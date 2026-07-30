# AGENTS.md

This file is durable project context for humans and coding agents. Read it before changing the repository.

## Project goal

`docker-vault-injector` is a small reconciliation controller for Docker Swarm. It observes **service-level** `deploy.labels`, reads HashiCorp Vault KV v2 values, and writes selected values to `ServiceSpec.TaskTemplate.ContainerSpec.Env`. Docker Swarm performs task replacement and rollout.

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

## Public label contract

User-controlled:

- `io.github.docker-vault-injector.enabled`
- `io.github.docker-vault-injector.secrets`

Controller-controlled:

- `io.github.docker-vault-injector.applied-versions`
- `io.github.docker-vault-injector.managed-env`
- `io.github.docker-vault-injector.state-hash`

All constants and parsing live in `internal/labels`. Do not duplicate literal label strings elsewhere.

The `secrets` label is a JSON object keyed by an arbitrary source name:

```json
{
  "database": {
    "mount": "kv",
    "path": "apps/api/database",
    "env": {
      "DB_USER": "username",
      "DB_PASSWORD": "credentials.password"
    }
  }
}
```

Changing this schema is a public API change. Maintain backwards compatibility or document a migration.

## Reconciliation algorithm

For an enabled service:

1. Parse and validate labels.
2. Parse controller-owned applied versions, managed names, and state hash.
3. Read `current_version` metadata for every configured source.
4. Compare versions, desired managed names, and the hash of current managed environment values.
5. If all match, return without reading Vault data or updating Docker.
6. Otherwise read the exact current version of every source.
7. Resolve dotted field paths and reject null/object/array values.
8. Remove previously managed keys, preserve unrelated environment entries, append desired keys in sorted order.
9. Update controller state labels.
10. Call one `ServiceUpdate` with the version from the inspected service.

For a disabled service with controller state, remove previously managed environment keys and controller state labels. Keep the user's `enabled=false` label.

## Code map

- `cmd/docker-vault-injector/main.go`: dependency wiring, logging, signal handling.
- `internal/config`: process-level environment settings only.
- `internal/controller`: orchestration and reconciliation policy.
- `internal/dockerclient`: thin adapter over `github.com/moby/moby/client`.
- `internal/vaultclient`: thin adapter over `github.com/hashicorp/vault/api`.
- `internal/labels`: public schema, validation, state serialization.
- `internal/environment`: deterministic Env merge/removal/hash.
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

The module path `github.com/vyktory/docker-vault-injector` is provisional because the repository currently has no Git remote. Change the module declaration and all internal imports together after the final GitHub owner is known.

## Known limitations and likely next steps

- Vault token is read once at startup; no AppRole login or token renewal yet.
- One controller replica is expected. Multiple replicas are mostly protected by Docker optimistic locking but can cause noisy conflicts; implement leader election before recommending HA replicas.
- Reapplying a stack file can cause one rollout from `docker stack deploy` and a second rollout from reinjection.
- There is no Prometheus metrics or HTTP health endpoint.
- Docker warnings returned after successful ServiceUpdate are currently ignored by the thin adapter.
- Full end-to-end tests against a real single-node Swarm and Vault dev server are not present.

When implementing these, preserve the small explicit architecture and update both README.md and this file.

