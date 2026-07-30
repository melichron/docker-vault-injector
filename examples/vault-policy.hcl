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
