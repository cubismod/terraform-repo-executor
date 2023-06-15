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

## Config file
The application processes the yaml/json defined at `CONFIG_FILE` for determining targets. The attributes are as follows:

| Attribute                | Type    | Description                                                                                                       |
|--------------------------|---------|-------------------------------------------------------------------------------------------------------------------|
| `dry_run`                | boolean | If `true`, the application executes `terraform plan`; if `false`, the application executes `terraform apply`.    |
| `repos`                | list(object) | A list of tf-repo targets. Below attributes comprise a tf-repo object.    |
| &emsp;`repository`       | string  | URL of the Git repository.                                                                                    |
| &emsp;`name`             | string  | A custom name for the repository, used as an identifier throughout the application.                               |
| &emsp;`ref`              | string  | Commit sha in the repository to be targeted.                           |
| &emsp;`project_path`     | string  | Relative path to the terraform workspace within the repository.                                                           |
| &emsp;`delete`           | boolean | If `true`, the application will execute the terraform action with the `destroy` flag set                |
| &emsp;`secret`     | object  | Vault secret where the terraform credentials for specified account are stored.                        |
| &emsp;&emsp;`path` | string  | Path to the secret in the vault. For KV v2, do not include the hidden `data` path segment                                                                               |
| &emsp;&emsp;`version` | integer | Version of the secret to be used.                                                                              |

### Example
``` 
dry_run: true
repos: 
- repository: https://gitlab.myinstance.com/some-gl-group/project_a
  name: foo-foo
  ref: d82b3cb292d91ec2eb26fc282d751555088819f3
  project_path: prod/networking
  delete: false
  secret:
    path: terraform/creds/prod-acount
    version: 4
- repository: https://gitlab.myinstance.com/another-gl-group/project_b
  name: bar-bar
  ref: 47ef09135da2d158ede78dbbe8c59de1775a274c
  project_path: stage/rds
  delete: false
  secret:
    path: terraform/creds/stage-account
    version: 1
```
