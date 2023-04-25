docker run \
    --network=host \
    -e GITLAB_USERNAME=$GITLAB_USERNAME \
    -e GITLAB_TOKEN=$GITLAB_TOKEN \
    -e VAULT_ADDR=$VAULT_ADDR \
    -e VAULT_ROLE_ID=$VAULT_ROLE_ID \
    -e VAULT_SECRET_ID=$VAULT_SECRET_ID \
    -v $(realpath config.yaml):/config.yaml:z \
    localhost/tf-repo-executor:latest