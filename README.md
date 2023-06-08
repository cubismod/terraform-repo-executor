# Terraform-Repo Executor

An application for performing terraform operations of target git repositories.

# Configuration 

## Environment Variables
### Required
* `VAULT_ADDR`
* `VAULT_ROLE_ID`
* `VAULT_SECRET_ID`
### Optional
* `CONFIG_FILE` - defaults to `/config.yaml`
* `WORKDIR` - defaults to `/tf-repo`
* `INTERNAL_GIT_CA_PATH` - if using a custom CA for cloning git repos

## Config file
The application processes the yaml/json defined at `CONFIG_FILE` for determining targets. The attributes are as follows:

| Attribute                | Type    | Description                                                                                                       |
|--------------------------|---------|-------------------------------------------------------------------------------------------------------------------|
| `repository`       | string  | URL of the Git repository.                                                                                    |
| `name`             | string  | A custom name for the repository, used as an identifier throughout the application.                               |
| `ref`              | string  | Commit sha in the repository to be targeted.                           |
| `project_path`     | string  | Relative path to the terraform workspace within the repository.                                                           |
| `dry_run`                | boolean | If `true`, the application executes `terraform plan`; if `false`, the application executes `terraform apply`.    |
| `delete`           | boolean | If `true`, the application will execute the terraform action with the `destroy` flag set                |
| `secret`     | object  | Vault secret where the terraform credentials for specified account are stored.                        |
| &emsp;`path` | string  | Path to the secret in the vault. For KV v2, do not include the hidden `data` path segment                                                                               |
| &emsp;`version` | integer | Version of the secret to be used.                                                                              |

### Example
``` 
name: foo-foo
repository: https://gitlab.myinstance.com/some-gl-group/project_a
ref: d82b3cb292d91ec2eb26fc282d751555088819f3
project_path: prod/networking
dry_run: true
delete: false
secret:
  path: terraform/creds/prod-acount
  version: 4
```
