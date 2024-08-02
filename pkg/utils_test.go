package pkg

import (
	"fmt"
	"os"
	"testing"

	"github.com/app-sre/terraform-repo-executor/pkg/vaultutil"
	"github.com/lithammer/dedent"
	"github.com/stretchr/testify/assert"
)

// Validates ablity to process accepted data formats
func TestProcessConfig(t *testing.T) {
	working, _ := os.Getwd()

	t.Run("valid yaml returns no error and actual equals expected", func(t *testing.T) {
		raw := `
            dry_run: true
            repos:
            - repository: https://gitlab.myinstance.com/some-gl-group/project_a
              name: foo-foo
              ref: d82b3cb292d91ec2eb26fc282d751555088819f3
              project_path: prod/networking
              delete: false
              aws_creds:
                path: terraform/creds/prod-account
                version: 4
              bucket: app-sre
              region: us-east-1
              bucket_path: tf-repo
              tf_version: 1.5.7
              require_fips: true
              variables:
                inputs:
                  path: terraform/foo-foo/inputs
                  version: 2
                outputs:
                  path: terraform/foo-foo/outputs
		`
		cfgPath := fmt.Sprintf("%s/%s", working, "good.yml")
		os.WriteFile(cfgPath, []byte(dedent.Dedent(raw)), 0644)
		defer os.Remove(cfgPath)

		cfg, err := processConfig(cfgPath)
		assert.Nil(t, err)

		expected := Input{
			DryRun: true,
			Repos: []Repo{
				{
					URL:    "https://gitlab.myinstance.com/some-gl-group/project_a",
					Name:   "foo-foo",
					Ref:    "d82b3cb292d91ec2eb26fc282d751555088819f3",
					Path:   "prod/networking",
					Delete: false,
					AWSCreds: vaultutil.VaultSecret{
						Path:    "terraform/creds/prod-account",
						Version: 4,
					},
					Bucket:      "app-sre",
					BucketPath:  "tf-repo",
					Region:      "us-east-1",
					TfVersion:   "1.5.7",
					RequireFips: true,
					TfVariables: TfVariables{
						Inputs:  vaultutil.VaultSecret{Path: "terraform/foo-foo/inputs", Version: 2},
						Outputs: vaultutil.VaultSecret{Path: "terraform/foo-foo/outputs", Version: 0}},
				},
			},
		}
		assert.Equal(t, expected, *cfg)
	})

	t.Run("valid json returns no error and actual matches expected", func(t *testing.T) {
		raw := `{
            "dry_run": true,
            "repos": [
              {
                "repository": "https://gitlab.myinstance.com/some-gl-group/project_a",
                "name": "foo-foo",
                "ref": "d82b3cb292d91ec2eb26fc282d751555088819f3",
                "project_path": "prod/networking",
                "delete": false,
                "aws_creds": {
                  "path": "terraform/creds/prod-acount",
                  "version": 4
                },
                "bucket": null,
                "bucket_path": null,
                "region": null,
                "tf_version": "1.5.7",
                "variables": {
                    "inputs": {
                        "path": "terraform/foo-foo/inputs",
                        "version": 2
                    },
                    "outputs": {
                        "path": "terraform/foo-foo/outputs"
                    }
                }
              },
              {
                "repository": "https://gitlab.myinstance.com/another-gl-group/project_b",
                "name": "bar-bar",
                "ref": "47ef09135da2d158ede78dbbe8c59de1775a274c",
                "project_path": "stage/network",
                "delete": true,
                "aws_creds": {
                  "path": "terraform/creds/stage-account",
                  "version": 1
                },
                "bucket": "app-sre",
                "bucket_path": "tf-repo",
                "region": "us-east-1",
                "tf_version": "1.4.7"
              }
            ]
}`
		cfgPath := fmt.Sprintf("%s/%s", working, "good.json")
		os.WriteFile(cfgPath, []byte(dedent.Dedent(raw)), 0644)
		defer os.Remove(cfgPath)

		cfg, err := processConfig(cfgPath)
		assert.Nil(t, err)

		expected := Input{
			DryRun: true,
			Repos: []Repo{
				{
					URL:    "https://gitlab.myinstance.com/some-gl-group/project_a",
					Name:   "foo-foo",
					Ref:    "d82b3cb292d91ec2eb26fc282d751555088819f3",
					Path:   "prod/networking",
					Delete: false,
					AWSCreds: vaultutil.VaultSecret{
						Path:    "terraform/creds/prod-acount",
						Version: 4,
					},
					Bucket:     "",
					BucketPath: "",
					Region:     "",
					TfVersion:  "1.5.7",
					TfVariables: TfVariables{
						Inputs:  vaultutil.VaultSecret{Path: "terraform/foo-foo/inputs", Version: 2},
						Outputs: vaultutil.VaultSecret{Path: "terraform/foo-foo/outputs", Version: 0}},
				},
				{
					URL:    "https://gitlab.myinstance.com/another-gl-group/project_b",
					Name:   "bar-bar",
					Ref:    "47ef09135da2d158ede78dbbe8c59de1775a274c",
					Path:   "stage/network",
					Delete: true,
					AWSCreds: vaultutil.VaultSecret{
						Path:    "terraform/creds/stage-account",
						Version: 1,
					},
					Bucket:     "app-sre",
					BucketPath: "tf-repo",
					Region:     "us-east-1",
					TfVersion:  "1.4.7",
				},
			},
		}
		assert.Equal(t, expected, *cfg)
	})

	t.Run("invalid yaml returns error", func(t *testing.T) {
		raw := `
			dry_run: true
			repos: 
			repository: https://gitlab.myinstance.com/some-gl-group/project_a
			name: foo-foo
			ref: d82b3cb292d91ec2eb26fc282d751555088819f3
			project_path: prod/networking
			delete: false
			secret:
				path: terraform/creds/prod-acount
				version: 4
		`
		cfgPath := fmt.Sprintf("%s/%s", working, "bad.yml")
		os.WriteFile(cfgPath, []byte(dedent.Dedent(raw)), 0644)
		defer os.Remove(cfgPath)

		_, err := processConfig(cfgPath)
		if err == nil {
			t.Fatal(fmt.Errorf("Invalid payload should result in error"))
		}
	})
}

func TestMaskingOfVaultValues(t *testing.T) {
	t.Run("values are masked correctly", func(t *testing.T) {
		input := `
			# aws_athena_database.vault:
			resource "aws_athena_database" "vault" {
				bucket        = "app-sre-athena-vault-output"
				force_destroy = false
				id            = "app_sre_vault"
				name          = "app_sre_vault"
				properties    = {}
			}

			# data.vault_generic_secret.bigsecret
			data "vault_generic_secret" "bigsecret" {
				data                  = {
					"fake_sensitive_cred" = "OHNO"
				}
				data_json             = jsonencode(
					{
						fake_sensitive_cred = "OHNO"
					}
				)
				id                    = "terraform-repo/input/athena/sensitivesecret"
				lease_duration        = 0
				lease_renewable       = false
				lease_start_time      = "2024-08-01T19:44:57Z"
				path                  = "terraform-repo/input/athena/sensitivesecret"
				version               = 1
				with_lease_start_time = true
			}

			# data.vault_generic_secret.bigsecret2
			data "vault_generic_secret" "bigsecret2" {
				data                  = {
					"fake_sensitive_cred" = "UHOH"
				}
				data_json             = jsonencode(
					{
						fake_sensitive_cred = "UHOH"
					}
				)
				id                    = "terraform-repo/input/athena/sensitivesecret2"
				lease_duration        = 0
				lease_renewable       = false
				lease_start_time      = "2024-08-01T19:44:57Z"
				path                  = "terraform-repo/input/athena/sensitivesecret2"
				version               = 1
				with_lease_start_time = true
			}`

		expected := `
			# aws_athena_database.vault:
			resource "aws_athena_database" "vault" {
				bucket        = "app-sre-athena-vault-output"
				force_destroy = false
				id            = "app_sre_vault"
				name          = "app_sre_vault"
				properties    = {}
			}

			# data.vault_generic_secret.bigsecret
			[REDACTED VAULT SECRET]

			# data.vault_generic_secret.bigsecret2
			[REDACTED VAULT SECRET]`

		actual := MaskSensitiveStateValues(dedent.Dedent(input))
		assert.Equal(t, dedent.Dedent(expected), actual)
	})

}
