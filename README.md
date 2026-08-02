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
