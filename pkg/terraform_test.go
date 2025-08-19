package pkg

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/app-sre/terraform-repo-executor/pkg/vaultutil"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const (
	accessKey   = "fake_access_key"
	secretKey   = "fake_secret_key"
	region      = "us-east-1"
	bucket      = "app-sre"
	repoName    = "a-repo"
	repoURL     = "https://gitlab.myinstance.com/some-gl-group/project_a"
	repoPath    = "prod/networking"
	repoRef     = "d82b3cb292d91ec2eb26fc282d751555088819f3"
	awsCredPath = "terraform/creds/prod-account"
	tfVersion   = "1.5.7"
)

var repoWithoutExplicitBucketSettings = Repo{
	Name:   repoName,
	URL:    repoURL,
	Path:   repoPath,
	Ref:    repoRef,
	Delete: false,
	AWSCreds: vaultutil.VaultSecret{
		Path:    awsCredPath,
		Version: 4,
	},
	RequireFips: false,
	TfVersion:   tfVersion,
}

func TestExtractTfCreds(t *testing.T) {
	exampleTfCreds := vaultutil.VaultKvData{
		AwsAccessKeyID:     accessKey,
		AwsSecretAccessKey: secretKey,
		AwsRegion:          region,
		AwsBucket:          bucket,
	}
	t.Run("credentials are extracted successfully when all fields present", func(t *testing.T) {
		expected := TfCreds{
			AccessKey: accessKey,
			SecretKey: secretKey,
			Region:    region,
			Bucket:    bucket,
		}

		creds, err := extractTfCreds(exampleTfCreds, repoWithoutExplicitBucketSettings)

		assert.Nil(t, err)
		assert.Equal(t, expected, creds)
	})

	t.Run("bucket credentials are extracted successfully when explicitly set in the repo config", func(t *testing.T) {
		explicitRegion := "us-west-1"
		explicitBucket := "sre-of-apps"

		expected := TfCreds{
			AccessKey: accessKey,
			SecretKey: secretKey,
			Region:    explicitRegion,
			Bucket:    explicitBucket,
		}

		repoWithExplicitBucketSettings := Repo{
			Name:   repoName,
			URL:    repoURL,
			Path:   repoPath,
			Ref:    repoRef,
			Delete: false,
			AWSCreds: vaultutil.VaultSecret{
				Path:    awsCredPath,
				Version: 4,
			},
			RequireFips: false,
			TfVersion:   tfVersion,
			Bucket:      explicitBucket,
			Region:      explicitRegion,
		}

		creds, err := extractTfCreds(exampleTfCreds, repoWithExplicitBucketSettings)

		assert.Nil(t, err)
		assert.Equal(t, expected, creds)
	})

	t.Run("error out when vault secret is missing some required keys", func(t *testing.T) {
		secretMissingKeys := vaultutil.VaultKvData{
			AwsAccessKeyID:     accessKey,
			AwsSecretAccessKey: secretKey,
		}

		_, err := extractTfCreds(secretMissingKeys, repoWithoutExplicitBucketSettings)

		assert.Error(t, err)
	})

}

func TestWriteTemplate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "template")

	assert.Nil(t, err)

	// create the directory that would normally be created when cloning the repo
	repoDir := fmt.Sprintf("%s/%s/%s", tmpDir,
		repoWithoutExplicitBucketSettings.Name,
		repoWithoutExplicitBucketSettings.Path)
	err = os.MkdirAll(repoDir, FolderPerm)

	assert.Nil(t, err)

	defer os.RemoveAll(tmpDir)

	t.Run("generate a backend file successfully", func(t *testing.T) {
		backendCreds := TfCreds{
			AccessKey: accessKey,
			SecretKey: secretKey,
			Region:    region,
			Key:       "a-repo-tf-repo.tfstate",
			Bucket:    bucket,
		}
		body := `access_key = "{{.AccessKey}}"
			{{- "\n"}}secret_key = "{{.SecretKey}}"
			{{- "\n"}}region = "{{.Region}}"
			{{- "\n"}}key = "{{.Key}}"
			{{- "\n"}}bucket = "{{.Bucket}}"`

		err := WriteTemplate(backendCreds, body, fmt.Sprintf("%s/%s/%s/%s", tmpDir, repoWithoutExplicitBucketSettings.Name, repoWithoutExplicitBucketSettings.Path, BackendFile))

		assert.Nil(t, err)

		expected, err := os.ReadFile("../test/data/s3.tfbackend")
		assert.Nil(t, err)

		output, err := os.ReadFile(fmt.Sprintf("%s/%s", repoDir, BackendFile))
		assert.Nil(t, err)

		assert.Equal(t, string(expected), string(output))
	})

	t.Run("generate an input tfvars file successfully", func(t *testing.T) {
		inputSecret := vaultutil.VaultKvData{
			"foo": "bar",
			"bar": "bell",
		}
		body := `{{ range $k, $v := . }}{{ $k }} = "{{ $v }}"{{- "\n"}}{{ end }}`

		err := WriteTemplate(inputSecret, body, fmt.Sprintf("%s/%s/%s/%s", tmpDir, repoWithoutExplicitBucketSettings.Name, repoWithoutExplicitBucketSettings.Path, InputVarsFile))

		assert.Nil(t, err)

		expected, err := os.ReadFile("../test/data/input.auto.tfvars")
		assert.Nil(t, err)

		output, err := os.ReadFile(fmt.Sprintf("%s/%s", repoDir, InputVarsFile))
		assert.Nil(t, err)

		assert.Equal(t, string(expected), string(output))
	})
}

// MockTerraformExecutor is a testify mock for TerraformExecutor~ interface
type MockTerraformExecutor struct {
	mock.Mock
}

func (m *MockTerraformExecutor) StatePull(ctx context.Context, opts ...tfexec.StatePullOption) (string, error) {
	args := m.Called(ctx, opts)
	return args.String(0), args.Error(1)
}

func TestValidateS3Backend(t *testing.T) {
	executor := &Executor{}
	repo := Repo{Name: "test-repo"}

	t.Run("successful S3 backend validation", func(t *testing.T) {
		validS3State := `{
			"version": 4,
			"backend": {
				"type": "s3",
				"config": {
					"bucket": "test-bucket",
					"key": "terraform.tfstate",
					"region": "us-east-1"
				}
			}
		}`

		mockTerraform := new(MockTerraformExecutor)
		mockTerraform.On("StatePull", mock.Anything, mock.Anything).Return(validS3State, nil)

		err := executor.validateS3Backend(mockTerraform, repo)
		assert.Nil(t, err)
		mockTerraform.AssertExpectations(t)
	})

	t.Run("fails when StatePull returns error", func(t *testing.T) {
		mockTerraform := new(MockTerraformExecutor)
		mockTerraform.On("StatePull", mock.Anything, mock.Anything).Return("", fmt.Errorf("state pull failed"))

		err := executor.validateS3Backend(mockTerraform, repo)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to pull terraform state for repo 'test-repo'")
		assert.Contains(t, err.Error(), "state pull failed")
		mockTerraform.AssertExpectations(t)
	})

	t.Run("fails when state is empty", func(t *testing.T) {
		mockTerraform := new(MockTerraformExecutor)
		mockTerraform.On("StatePull", mock.Anything, mock.Anything).Return("", nil)

		err := executor.validateS3Backend(mockTerraform, repo)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "repository 'test-repo' has empty terraform state, which indicates no S3 backend is configured")
		mockTerraform.AssertExpectations(t)
	})

	t.Run("fails when state is invalid JSON", func(t *testing.T) {
		mockTerraform := new(MockTerraformExecutor)
		mockTerraform.On("StatePull", mock.Anything, mock.Anything).Return("invalid json", nil)

		err := executor.validateS3Backend(mockTerraform, repo)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse terraform state for repo 'test-repo'")
		mockTerraform.AssertExpectations(t)
	})

	t.Run("fails when backend configuration is missing", func(t *testing.T) {
		stateWithoutBackend := `{
			"version": 4,
			"resources": []
		}`

		mockTerraform := new(MockTerraformExecutor)
		mockTerraform.On("StatePull", mock.Anything, mock.Anything).Return(stateWithoutBackend, nil)

		err := executor.validateS3Backend(mockTerraform, repo)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "repository 'test-repo' does not have backend configuration in terraform state")
		mockTerraform.AssertExpectations(t)
	})

	t.Run("fails when backend type is missing", func(t *testing.T) {
		stateWithInvalidBackend := `{
			"version": 4,
			"backend": {
				"config": {
					"bucket": "test-bucket"
				}
			}
		}`

		mockTerraform := new(MockTerraformExecutor)
		mockTerraform.On("StatePull", mock.Anything, mock.Anything).Return(stateWithInvalidBackend, nil)

		err := executor.validateS3Backend(mockTerraform, repo)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "repository 'test-repo' has invalid backend type in terraform state")
		mockTerraform.AssertExpectations(t)
	})

	t.Run("fails when backend type is not s3", func(t *testing.T) {
		localBackendState := `{
			"version": 4,
			"backend": {
				"type": "local",
				"config": {
					"path": "terraform.tfstate"
				}
			}
		}`

		mockTerraform := new(MockTerraformExecutor)
		mockTerraform.On("StatePull", mock.Anything, mock.Anything).Return(localBackendState, nil)

		err := executor.validateS3Backend(mockTerraform, repo)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "repository 'test-repo' is using backend type 'local' instead of required S3 backend")
		mockTerraform.AssertExpectations(t)
	})
}
