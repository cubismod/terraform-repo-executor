package pkg

import (
	"testing"

	"github.com/app-sre/terraform-repo-executor/pkg/vaultutil"
	"github.com/stretchr/testify/assert"
)

// examples taken from https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html
const (
	accessKey   = "AKIAIOSFODNN7EXAMPLE"
	secretKey   = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	region      = "us-east-1"
	bucket      = "app-sre"
	repoName    = "a-repo"
	repoUrl     = "https://gitlab.myinstance.com/some-gl-group/project_a"
	repoPath    = "prod/networking"
	repoRef     = "d82b3cb292d91ec2eb26fc282d751555088819f3"
	awsCredPath = "terraform/creds/prod-account"
	tfVersion   = "1.5.7"
)

var repoWithoutExplicitBucketSettings = Repo{
	Name:   repoName,
	URL:    repoUrl,
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
	credsSecret := vaultutil.VaultKvData{
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

		creds, err := extractTfCreds(credsSecret, repoWithoutExplicitBucketSettings)

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
			URL:    repoUrl,
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

		creds, err := extractTfCreds(credsSecret, repoWithExplicitBucketSettings)

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
