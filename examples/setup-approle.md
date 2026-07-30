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
