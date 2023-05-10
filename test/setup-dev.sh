#! /bin/bash
kill "$(pidof vault)"

export VAULT_ADDR=http://localhost:8200
export VAULT_AUTHTYPE=token
export VAULT_TOKEN=root
export VAULT_FORMAT=json

vault server -dev -dev-root-token-id=root &

vault secrets enable -version=2 -path=terraform kv
vault kv put -mount=terraform app-sre/prod-network  name=linguinie
vault policy write tf-executor policy.hcl
vault auth enable approle
vault write auth/approle/role/tf-executor token_num_uses=999 policies="tf-executor, default"

export VAULT_ROLE_ID=$(vault read auth/approle/role/tf-executor/role-id | jq -r .data.role_id)
export VAULT_SECRET_ID=$(vault write -f auth/approle/role/tf-executor/secret-id | jq -r .data.secret_id)
