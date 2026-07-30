# docker-vault-injector

Небольшой reconciliation-controller для Docker Swarm. Он читает конфигурацию из labels сервиса, получает значения из HashiCorp Vault KV v2 и добавляет их в `ServiceSpec.TaskTemplate.ContainerSpec.Env`. При изменении секрета контроллер обновляет сервис, а Swarm выполняет обычный rolling update его задач.

Проект ориентирован и на полноценные кластеры, и на single-node Swarm. Одиночный Swarm можно создать одной командой:

```bash
docker swarm init
```

## Текущий статус

Это понятный, рабочий каркас MVP, а не законченный production-релиз. Уже реализованы:

- Docker service event stream для быстрой реакции на создание и изменение сервисов;
- периодический полный resync на случай пропущенных событий;
- чтение metadata и конкретной версии Vault KV v2;
- несколько Vault-документов для одного сервиса;
- вложенные поля вроде `connection.host`;
- optimistic locking через Docker `Version.Index`;
- сохранение всех не принадлежащих контроллеру полей `ServiceSpec`;
- обнаружение ручного изменения уже внедрённых environment variables;
- удаление старых managed variables при изменении mapping;
- очистка внедрённых variables при `enabled: "false"`;
- отсутствие значений секретов в логах контроллера;
- unit-тесты основных сценариев.

Пока не реализованы AppRole login, автоматическое продление Vault token, leader election для нескольких экземпляров контроллера, metrics/health endpoint и полноценные end-to-end тесты.

## Как это работает

```text
Docker service create/update event       Периодический timer
                 │                              │
                 └──────────┬───────────────────┘
                            ▼
                   inspect Swarm Service
                            │
                   прочитать deploy.labels
                            │
              получить current_version из Vault
                            │
                  версия и state hash прежние?
                     ┌──────┴──────┐
                    да            нет
                     │              │
                  ничего     прочитать точные версии
                                    │
                              построить Env
                                    │
                             ServiceUpdate
                                    │
                            Swarm rolling update
```

Контроллер сначала читает `current_version` из metadata. Если необходимо обновление, он запрашивает именно эту версию через `GetVersion`. Это исключает гонку, когда между чтением metadata и данных в Vault появляется ещё одна версия.

Помимо версии хранится SHA-256 hash управляемой части environment. Он позволяет заметить ручное изменение `DB_PASSWORD` в Docker ServiceSpec, даже если версия Vault не менялась. Hash не считается границей безопасности: пользователь, способный прочитать service labels, обычно может прочитать и `ServiceSpec.Env`.

## Labels сервиса

Labels должны находиться именно в `deploy.labels`. Обычный блок `labels` создаёт labels контейнеров, а контроллер наблюдает Swarm Services.

```yaml
services:
  api:
    image: registry.example.com/api:1.0.0
    deploy:
      labels:
        io.github.docker-vault-injector.enabled: "true"
        io.github.docker-vault-injector.secrets: |
          {
            "database": {
              "mount": "kv",
              "path": "apps/api/database",
              "env": {
                "DB_USER": "username",
                "DB_PASSWORD": "password"
              }
            },
            "redis": {
              "mount": "kv",
              "path": "apps/api/redis",
              "env": {
                "REDIS_PASSWORD": "credentials.password"
              }
            }
          }
```

`mount` — имя mount point KV v2 без `/data/` или `/metadata/`. `path` — логический путь внутри mount. В примере физические API endpoints будут `kv/metadata/apps/api/database` и `kv/data/apps/api/database`.

Имена верхнего уровня (`database`, `redis`) произвольны, но должны быть уникальны. Одна environment variable не может принадлежать двум источникам одновременно. В environment допускаются только скалярные Vault-значения: строки, числа и boolean. Объекты и массивы намеренно отклоняются.

Контроллер самостоятельно добавляет labels:

```text
io.github.docker-vault-injector.applied-versions
io.github.docker-vault-injector.managed-env
io.github.docker-vault-injector.state-hash
```

Их не следует редактировать вручную. Значений секретов в этих labels нет.

## Отключение

Чтобы контроллер удалил ранее внедрённые variables, установите:

```yaml
deploy:
  labels:
    io.github.docker-vault-injector.enabled: "false"
```

На следующем reconcile контроллер удалит variables из `managed-env`, очистит свои state labels и обновит сервис. После завершения cleanup labels можно удалить полностью.

## Требования Vault

Поддерживается только KV v2. Минимальная policy находится в [examples/vault-policy.hcl](examples/vault-policy.hcl):

```hcl
path "kv/metadata/apps/*" {
  capabilities = ["read"]
}

path "kv/data/apps/*" {
  capabilities = ["read"]
}
```

Пример данных:

```bash
vault kv put -mount=kv apps/example/database \
  username=example \
  password=change-me
```

Для вложенного объекта удобнее использовать JSON:

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

## Конфигурация процесса

Стандартные variables официального Vault Go client поддерживаются: `VAULT_ADDR`, `VAULT_CACERT`, `VAULT_CAPATH`, `VAULT_CLIENT_CERT`, `VAULT_CLIENT_KEY`, `VAULT_NAMESPACE` и proxy variables.

| Variable | Default | Назначение |
|---|---:|---|
| `VAULT_TOKEN` | — | Vault token непосредственно в environment |
| `VAULT_TOKEN_FILE` | — | Файл с Vault token; имеет приоритет над `VAULT_TOKEN` |
| `INJECTOR_POLL_INTERVAL` | `30s` | Интервал полного resync |
| `INJECTOR_RECONCILE_TIMEOUT` | `20s` | Timeout одного Docker/Vault reconcile |
| `INJECTOR_EVENT_RETRY_INTERVAL` | `5s` | Задержка перед переподключением к Docker events |
| `INJECTOR_LOG_LEVEL` | `info` | `debug`, `info`, `warn` или `error` |

Token читается при запуске и пока автоматически не обновляется. Для MVP используйте периодически возобновляемый token с минимальной policy и перезапускайте injector при его замене. AppRole/auto-auth — логичное следующее расширение.

## Сборка и тесты

Требуется Go 1.26 или новее.

```bash
make test
make vet
make build
```

Сборка контейнера:

```bash
make docker-build
```

Если module path будущего GitHub-репозитория отличается от указанного в `go.mod`, замените его до первой публикации:

```bash
go mod edit -module github.com/OWNER/docker-vault-injector
rg -l 'github.com/vyktory/docker-vault-injector' --glob '*.go' \
  | xargs sed -i 's#github.com/vyktory/docker-vault-injector#github.com/OWNER/docker-vault-injector#g'
```

## Запуск в Swarm

Сначала создайте token как Docker Secret:

```bash
printf '%s' "$VAULT_TOKEN" | docker secret create vault_injector_token -
```

Соберите image и разверните пример:

```bash
docker build -t docker-vault-injector:dev .
docker stack deploy -c examples/stack.yaml vault-injector-demo
```

Injector должен работать на manager node, потому что Service API является manager API. В примере это обеспечивается placement constraint:

```yaml
deploy:
  replicas: 1
  placement:
    constraints:
      - node.role==manager
```

Проверьте результат:

```bash
docker service inspect vault-injector-demo_example \
  --format '{{json .Spec.TaskTemplate.ContainerSpec.Env}}'
```

После записи новой версии:

```bash
vault kv patch -mount=kv apps/example/database password=new-password
```

контроллер заметит новый `current_version`, изменит environment и Swarm заменит task сервиса.

## Поведение при ошибках

Fail-safe правило проекта: **никогда не заменять рабочие значения пустыми из-за ошибки Vault или конфигурации**.

Если Vault недоступен, версия удалена, field отсутствует или mapping некорректен, ServiceSpec остаётся без изменений, а ошибка записывается в лог без secret values. Следующий Docker event или периодический resync повторит попытку.

Если между `ServiceInspect` и `ServiceUpdate` другой процесс изменил сервис, Docker отклонит устаревший `Version.Index`. Контроллер не перезаписывает более новое состояние; следующий event/resync выполнит reconcile заново.

## Взаимодействие с `docker stack deploy`

Повторный `docker stack deploy` может убрать environment и state labels, добавленные контроллером, потому что их нет в исходном stack YAML. Injector восстановит их после service update event. Это может вызвать второй rolling update сразу после rollout самого stack deploy.

Для MVP такое eventual-consistency поведение считается допустимым. Если потребуется ровно один rollout, понадобится отдельный deploy wrapper/preprocessor, а это другой scope.

## Безопасность

Контейнер имеет доступ к `/var/run/docker.sock`, то есть фактически обладает административными правами на Docker host/Swarm. Запускайте только доверенный image и не публикуйте Docker API наружу без TLS.

Секреты находятся в `ServiceSpec.Env` и видны через `docker service inspect` пользователям с правами управления Swarm. Это сознательная модель проекта: тот же пользователь способен выполнить команду внутри task-контейнера и прочитать его environment.

Контроллер никогда не пишет secret values в собственные логи. Не включайте их в labels и не используйте значения секретов в именах Vault paths.

## Структура проекта

```text
cmd/docker-vault-injector/   wiring, signals, process lifecycle
internal/config/             process environment configuration
internal/controller/         reconciliation and event loop
internal/dockerclient/       thin Moby client adapter
internal/environment/        merge, cleanup and drift hash
internal/labels/             public label schema and validation
internal/vaultclient/        thin official Vault client adapter
examples/                    Swarm stack and Vault policy
```

Подробные договорённости для будущих разработчиков и coding agents находятся в [AGENTS.md](AGENTS.md).
