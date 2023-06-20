package pkg

import (
	"fmt"
	"os"
	"testing"

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
  secret:
    path: terraform/creds/prod-acount
    version: 4
`
		cfgPath := fmt.Sprintf("%s/%s", working, "good.yml")
		os.WriteFile(cfgPath, []byte(raw), 0644)
		defer os.Remove(cfgPath)

		cfg, err := processConfig(cfgPath)
		assert.Nil(t, err)

		expected := TfRepo{
			DryRun: true,
			Repos: []Repo{
				{
					Url:    "https://gitlab.myinstance.com/some-gl-group/project_a",
					Name:   "foo-foo",
					Ref:    "d82b3cb292d91ec2eb26fc282d751555088819f3",
					Path:   "prod/networking",
					Delete: false,
					Secret: VaultSecret{
						Path:    "terraform/creds/prod-acount",
						Version: 4,
					},
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
				"secret": {
				  "path": "terraform/creds/prod-acount",
				  "version": 4
				}
			  },
			  {
				"repository": "https://gitlab.myinstance.com/another-gl-group/project_b",
				"name": "bar-bar",
				"ref": "47ef09135da2d158ede78dbbe8c59de1775a274c",
				"project_path": "stage/network",
				"delete": true,
				"secret": {
				  "path": "terraform/creds/stage-account",
				  "version": 1
				}
			  }
			]
}`
		cfgPath := fmt.Sprintf("%s/%s", working, "good.json")
		os.WriteFile(cfgPath, []byte(raw), 0644)
		defer os.Remove(cfgPath)

		cfg, err := processConfig(cfgPath)
		assert.Nil(t, err)

		expected := TfRepo{
			DryRun: true,
			Repos: []Repo{
				{
					Url:    "https://gitlab.myinstance.com/some-gl-group/project_a",
					Name:   "foo-foo",
					Ref:    "d82b3cb292d91ec2eb26fc282d751555088819f3",
					Path:   "prod/networking",
					Delete: false,
					Secret: VaultSecret{
						Path:    "terraform/creds/prod-acount",
						Version: 4,
					},
				},
				{
					Url:    "https://gitlab.myinstance.com/another-gl-group/project_b",
					Name:   "bar-bar",
					Ref:    "47ef09135da2d158ede78dbbe8c59de1775a274c",
					Path:   "stage/network",
					Delete: true,
					Secret: VaultSecret{
						Path:    "terraform/creds/stage-account",
						Version: 1,
					},
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
		os.WriteFile(cfgPath, []byte(raw), 0644)
		defer os.Remove(cfgPath)

		_, err := processConfig(cfgPath)
		if err == nil {
			t.Fatal(fmt.Errorf("Invalid payload should result in error"))
		}
	})
}
