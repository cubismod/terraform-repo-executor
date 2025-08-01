package pkg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTerraformRepo_ValidConfiguration(t *testing.T) {
	tmpDir := t.TempDir()

	validTerraform := `
terraform {
  backend "s3" {}
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.0"
    }
  }
}

variable "access_key" {}
variable "secret_key" {}
variable "region" {}

provider "aws" {
  region = var.region
  access_key = var.access_key
  secret_key = var.secret_key
}
`

	err := os.WriteFile(filepath.Join(tmpDir, "providers.tf"), []byte(validTerraform), 0644)
	require.NoError(t, err)

	config := GetDefaultValidationConfig()
	result := ValidateTerraformRepo(tmpDir, config)

	assert.True(t, result.Valid, "Expected configuration to be valid")
	assert.Empty(t, result.Errors, "Expected no validation errors")
}

func TestValidateTerraformRepo_MissingTerraformBlock(t *testing.T) {
	tmpDir := t.TempDir()

	invalidTerraform := `
variable "access_key" {}
variable "secret_key" {}
variable "region" {}

provider "aws" {
  region = var.region
  access_key = var.access_key
  secret_key = var.secret_key
}
`

	err := os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(invalidTerraform), 0644)
	require.NoError(t, err)

	config := GetDefaultValidationConfig()
	result := ValidateTerraformRepo(tmpDir, config)

	assert.False(t, result.Valid, "Expected configuration to be invalid")
	assert.NotEmpty(t, result.Errors, "Expected validation errors")

	// Check for specific error about missing terraform block
	found := false
	for _, err := range result.Errors {
		if strings.Contains(err.Message, "No terraform block found") {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected error about missing terraform block")
}

func TestValidateTerraformRepo_MissingS3Backend(t *testing.T) {
	tmpDir := t.TempDir()

	invalidTerraform := `
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.0"
    }
  }
}

variable "access_key" {}
variable "secret_key" {}
variable "region" {}
`

	err := os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(invalidTerraform), 0644)
	require.NoError(t, err)

	config := GetDefaultValidationConfig()
	result := ValidateTerraformRepo(tmpDir, config)

	assert.False(t, result.Valid, "Expected configuration to be invalid")

	// Check for specific error about missing S3 backend
	found := false
	for _, err := range result.Errors {
		if strings.Contains(err.Message, "S3 backend not found") {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected error about missing S3 backend")
}

func TestValidateTerraformRepo_MissingRequiredVariables(t *testing.T) {
	tmpDir := t.TempDir()

	invalidTerraform := `
terraform {
  backend "s3" {}
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.0"
    }
  }
}

variable "access_key" {}
# missing secret_key variable
`

	err := os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(invalidTerraform), 0644)
	require.NoError(t, err)

	config := GetDefaultValidationConfig()
	result := ValidateTerraformRepo(tmpDir, config)

	assert.False(t, result.Valid, "Expected configuration to be invalid")

	// Check for specific error about missing secret_key variable
	secretKeyFound := false
	for _, err := range result.Errors {
		if strings.Contains(err.Message, "Required variable 'secret_key' not found") {
			secretKeyFound = true
			break
		}
	}
	assert.True(t, secretKeyFound, "Expected error about missing secret_key variable")
}

func TestValidateTerraformRepo_MissingRequiredProvider(t *testing.T) {
	tmpDir := t.TempDir()

	invalidTerraform := `
terraform {
  backend "s3" {}
  required_providers {
    # aws provider is missing
  }
}

variable "access_key" {}
variable "secret_key" {}
variable "region" {}
`

	err := os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(invalidTerraform), 0644)
	require.NoError(t, err)

	config := GetDefaultValidationConfig()
	result := ValidateTerraformRepo(tmpDir, config)

	assert.False(t, result.Valid, "Expected configuration to be invalid")

	// Check for specific error about missing AWS provider
	found := false
	for _, err := range result.Errors {
		if strings.Contains(err.Message, "Required provider 'aws' not found in required_providers block") {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected error about missing AWS provider")
}

func TestValidateTerraformRepo_MultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Split configuration across multiple files
	terraformFile := `
terraform {
  backend "s3" {}
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.0"
    }
  }
}
`

	variablesFile := `
variable "access_key" {}
variable "secret_key" {}
variable "region" {}
`

	providerFile := `
provider "aws" {
  region = var.region
  access_key = var.access_key
  secret_key = var.secret_key
}
`

	err := os.WriteFile(filepath.Join(tmpDir, "terraform.tf"), []byte(terraformFile), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "variables.tf"), []byte(variablesFile), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "providers.tf"), []byte(providerFile), 0644)
	require.NoError(t, err)

	config := GetDefaultValidationConfig()
	result := ValidateTerraformRepo(tmpDir, config)

	assert.True(t, result.Valid, "Expected configuration to be valid across multiple files")
	assert.Empty(t, result.Errors, "Expected no validation errors")
}

func TestValidateTerraformRepo_NoTerraformFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create non-terraform file
	err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Test"), 0644)
	require.NoError(t, err)

	config := GetDefaultValidationConfig()
	result := ValidateTerraformRepo(tmpDir, config)

	assert.False(t, result.Valid, "Expected configuration to be invalid")

	found := false
	for _, err := range result.Errors {
		if strings.Contains(err.Message, "No .tf files found in repository") {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected error about no .tf files found")
}

func TestValidateTerraformRepo_CustomConfig(t *testing.T) {
	tmpDir := t.TempDir()

	validTerraform := `
terraform {
  backend "s3" {}
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
    }
  }
}

variable "access_key" {}
variable "secret_key" {}
variable "custom_var" {}
`

	err := os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(validTerraform), 0644)
	require.NoError(t, err)

	// Custom config with additional requirements
	config := ValidationConfig{
		RequiredVariables: []RequiredVariable{
			{Name: "access_key"},
			{Name: "secret_key"},
			{Name: "custom_var"},
		},
		RequiredProviders: []RequiredProvider{
			{LocalName: "aws", Source: "hashicorp/aws"},
			{LocalName: "random", Source: "hashicorp/random"},
		},
	}

	result := ValidateTerraformRepo(tmpDir, config)

	assert.True(t, result.Valid, "Expected configuration to be valid with custom config")
	assert.Empty(t, result.Errors, "Expected no validation errors")
}

func TestValidateTerraformRepo_AWSProviderWithAlias(t *testing.T) {
	tmpDir := t.TempDir()

	validTerraform := `
terraform {
  backend "s3" {}
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.0"
    }
  }
}

variable "access_key" {}
variable "secret_key" {}

provider "aws" {
  region = "us-east-1"
}

provider "aws" {
  alias  = "west"
  region = "us-west-2"
}
`

	err := os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(validTerraform), 0644)
	require.NoError(t, err)

	config := GetDefaultValidationConfig()
	result := ValidateTerraformRepo(tmpDir, config)

	assert.True(t, result.Valid, "Expected configuration with AWS provider aliases to be valid")
	assert.Empty(t, result.Errors, "Expected no validation errors")
}
