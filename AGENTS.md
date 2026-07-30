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

## Public label contract

User-controlled:

- `io.github.docker-vault-injector.enabled`
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

The old multiline `io.github.docker-vault-injector.secrets` JSON label is not supported. This is currently a pre-release project, so the flat schema intentionally replaced it instead of adding a compatibility parser. Any future schema change is a public API change and needs an explicit migration story.

## Reconciliation algorithm

For an enabled service:

1. Parse and validate labels.
2. Parse controller-owned applied versions, managed names, state hash, and config hash.
3. Read `current_version` metadata for every configured source.
4. Compare versions, configuration hash, and the hash of current previously-managed environment values.
5. If all match, return without reading Vault data or updating Docker.
6. Otherwise read the exact current version of every source.
7. Auto-import all top-level scalar fields for sources without mappings; otherwise resolve the explicit dotted field paths.
8. Reject null/object/array values and reject duplicate environment ownership across every source.
9. Remove previously managed keys, preserve unrelated environment entries, append desired keys in sorted order.
10. Update controller state labels.
11. Call one `ServiceUpdate` with the version from the inspected service.

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

- AppRole uses reusable RoleID/SecretID credentials; response-wrapped SecretID delivery is not implemented yet.
- One controller replica is expected. Multiple replicas are mostly protected by Docker optimistic locking but can cause noisy conflicts; implement leader election before recommending HA replicas.
- Reapplying a stack file can cause one rollout from `docker stack deploy` and a second rollout from reinjection.
- There is no Prometheus metrics or HTTP health endpoint.
- Docker warnings returned after successful ServiceUpdate are currently ignored by the thin adapter.
- Full end-to-end tests against a real single-node Swarm and Vault dev server are not present.

When implementing these, preserve the small explicit architecture and update both README.md and this file.
