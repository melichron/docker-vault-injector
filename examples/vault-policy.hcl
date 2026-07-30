# The injector polls KV v2 metadata and reads an exact data version only when
# the version or the current Docker environment state has changed.
path "kv/metadata/apps/*" {
  capabilities = ["read"]
}

path "kv/data/apps/*" {
  capabilities = ["read"]
}

