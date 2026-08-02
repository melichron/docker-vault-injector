# Creating a Vault policy and AppRole

This example creates a dedicated AppRole for `docker-vault-injector` using the custom auth mount `auth/docker-swarm`. The injector must be configured with the full mount path `auth/docker-swarm`, without the `/login` suffix.

Run these commands as a Vault operator with permission to create auth methods, policies, and AppRoles. Adjust the values to match your infrastructure:

```bash
export VAULT_ADDR=https://vault.example.com:8200
export AUTH_PATH=docker-swarm
export ROLE_NAME=docker-vault-injector
export POLICY_NAME=docker-vault-injector
```

For the Vault CLI, `AUTH_PATH` contains the mount name without the `auth/` prefix. The same mount is configured in the injector as `VAULT_APPROLE_AUTH_PATH=auth/docker-swarm`.

## 1. Enable the AppRole auth method

```bash
vault auth enable -path="$AUTH_PATH" approle
```

Skip this step if the auth method already exists. To list the existing auth mounts, run:

```bash
vault auth list
```

## 2. Create the policy

A ready-to-use policy file is provided alongside this document: [vault-policy.hcl](vault-policy.hcl).

```bash
vault policy write "$POLICY_NAME" examples/vault-policy.hcl
```

The policy permits the injector to:

- read KV metadata and data under `kv/apps/*`;
- inspect its current token through `auth/token/lookup-self`;
- renew its token through `auth/token/renew-self`.

If the KV v2 engine is mounted somewhere other than `kv`, or the secrets are stored outside `apps/*`, update both KV paths in the policy.

## 3. Create the AppRole

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

This example deliberately uses a periodic service token:

- `token_period=20m` issues a renewable token with a 20-minute TTL;
- the injector checks the token status and renews it after approximately two-thirds of its TTL;
- if the token is revoked or renewal fails, the injector performs another AppRole login;
- `secret_id_num_uses=0` allows repeated logins with the same SecretID;
- `secret_id_ttl=0` prevents the SecretID from expiring on its own.

A non-expiring, reusable SecretID is convenient for initial operation, but it is a long-lived credential. Store it as a Docker Secret, restrict the policy to the minimum required paths, and rotate it periodically. A stricter deployment can use a short-lived response-wrapped SecretID, but that requires a separate delivery and refresh mechanism.

## 4. Obtain the RoleID and SecretID

```bash
vault read -field=role_id \
  "auth/$AUTH_PATH/role/$ROLE_NAME/role-id" \
  > role-id.txt

vault write -field=secret_id -f \
  "auth/$AUTH_PATH/role/$ROLE_NAME/secret-id" \
  > secret-id.txt
```

Never print the SecretID in CI logs. Securely remove the local files after creating the Docker Secrets.

## 5. Provide the credentials to Docker Swarm

```bash
docker secret create vault_approle_role_id role-id.txt
docker secret create vault_approle_secret_id secret-id.txt
```

Then use [stack.yaml](stack.yaml), where the credentials are mounted at:

```text
/run/secrets/vault_approle_role_id
/run/secrets/vault_approle_secret_id
```

The injector re-reads both files before every new AppRole login. To rotate Docker Secrets, create new versioned secrets and mount them at the same target paths.

## 6. Test the AppRole manually

For diagnostic purposes, perform a login manually:

```bash
vault write "auth/$AUTH_PATH/login" \
  role_id="$(tr -d '\n' < role-id.txt)" \
  secret_id="$(tr -d '\n' < secret-id.txt)"
```

The response should contain a `token_duration` of approximately `20m` and `token_renewable=true`.
