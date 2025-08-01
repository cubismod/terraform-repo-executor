package pkg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
)

const TerraformRepoTemplateURL = "https://gitlab.cee.redhat.com/app-sre/terraform-repo-template"

type ValidationError struct {
	Message string
	File    string
}

func (ve ValidationError) Error() string {
	if ve.File != "" {
		return fmt.Sprintf("%s (in %s)", ve.Message, ve.File)
	}
	return ve.Message
}

type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

type RequiredVariable struct {
	Name string
}

// RequiredProvider represents a provider that must be present in required_providers
type RequiredProvider struct {
	LocalName string // e.g. "aws"
	Source    string // e.g. "hashicorp/aws"
}

// fileWithPath pairs an HCL file with its filesystem path
type fileWithPath struct {
	file *hcl.File
	path string
}

// ValidationConfig defines what should be validated
type ValidationConfig struct {
	RequiredVariables []RequiredVariable
	RequiredProviders []RequiredProvider
}

// GetDefaultValidationConfig returns the standard validation configuration for tf-repo
func GetDefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		RequiredVariables: []RequiredVariable{
			{Name: "access_key"},
			{Name: "secret_key"},
		},
		RequiredProviders: []RequiredProvider{
			{LocalName: "aws", Source: "hashicorp/aws"},
		},
	}
}

// ValidateTerraformRepo validates that a terraform repository meets tf-repo requirements
func ValidateTerraformRepo(repoPath string, config ValidationConfig) ValidationResult {
	result := ValidationResult{
		Valid:  true,
		Errors: []ValidationError{},
	}

	tfFiles, err := findTerraformFiles(repoPath)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Message: fmt.Sprintf("Error finding terraform files: %v", err),
		})
		return result
	}

	if len(tfFiles) == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Message: "No .tf files found in repository",
		})
		return result
	}

	parser := hclparse.NewParser()
	var allFiles []fileWithPath

	for _, tfFile := range tfFiles {
		content, err := os.ReadFile(tfFile)
		if err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Message: fmt.Sprintf("Error reading file: %v", err),
				File:    tfFile,
			})
			continue
		}

		file, diags := parser.ParseHCL(content, tfFile)
		if diags.HasErrors() {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Message: fmt.Sprintf("HCL parsing error: %s", diags.Error()),
				File:    tfFile,
			})
			continue
		}
		allFiles = append(allFiles, fileWithPath{file: file, path: tfFile})
	}

	// Validate requirements across all files
	validationErrors := validateRequirements(allFiles, config)
	result.Errors = append(result.Errors, validationErrors...)

	if len(validationErrors) > 0 {
		result.Valid = false
	}

	return result
}

// findTerraformFiles finds all .tf files in the given directory
func findTerraformFiles(dir string) ([]string, error) {
	var tfFiles []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".tf") {
			tfFiles = append(tfFiles, path)
		}

		return nil
	})

	return tfFiles, err
}

// validateRequirements checks that all required elements are present
func validateRequirements(files []fileWithPath, config ValidationConfig) []ValidationError {
	var errors []ValidationError

	// Track what we've found
	foundVariables := make(map[string]bool)
	foundProviders := make(map[string]bool)
	foundS3Backend := false
	foundTerraformBlock := false

	// Parse each file
	for _, fileInfo := range files {
		content, _, diags := fileInfo.file.Body.PartialContent(&hcl.BodySchema{
			Blocks: []hcl.BlockHeaderSchema{
				{Type: "terraform"},
				{Type: "variable", LabelNames: []string{"name"}},
				{Type: "provider", LabelNames: []string{"name"}},
			},
		})

		if diags.HasErrors() {
			errors = append(errors, ValidationError{
				Message: fmt.Sprintf("Error parsing HCL structure: %s", diags.Error()),
				File:    fileInfo.path,
			})
			continue
		}

		// Check terraform blocks
		for _, block := range content.Blocks {
			switch block.Type {
			case "terraform":
				foundTerraformBlock = true
				terraformContent, _, _ := block.Body.PartialContent(&hcl.BodySchema{
					Blocks: []hcl.BlockHeaderSchema{
						{Type: "backend", LabelNames: []string{"type"}},
						{Type: "required_providers"},
					},
				})

				// Check for S3 backend
				for _, backendBlock := range terraformContent.Blocks {
					if backendBlock.Type == "backend" && len(backendBlock.Labels) > 0 && backendBlock.Labels[0] == "s3" {
						foundS3Backend = true
					}
				}

				// Check required_providers
				for _, reqProvidersBlock := range terraformContent.Blocks {
					if reqProvidersBlock.Type == "required_providers" {
						attrs, _ := reqProvidersBlock.Body.JustAttributes()
						for _, provider := range config.RequiredProviders {
							if _, exists := attrs[provider.LocalName]; exists {
								foundProviders[provider.LocalName] = true
							}
						}
					}
				}

			case "variable":
				if len(block.Labels) > 0 {
					foundVariables[block.Labels[0]] = true
				}

			case "provider":
				if len(block.Labels) > 0 {
					foundProviders[block.Labels[0]] = true
				}
			}
		}
	}

	if !foundTerraformBlock {
		errors = append(errors, ValidationError{
			Message: fmt.Sprintf("No terraform block found - required for backend and provider configuration. See %s for the expected structure", TerraformRepoTemplateURL),
		})
	}

	if !foundS3Backend {
		errors = append(errors, ValidationError{
			Message: fmt.Sprintf(`S3 backend not found - terraform block must include: backend "s3" {}. See %s for the expected structure`, TerraformRepoTemplateURL),
		})
	}

	for _, reqVar := range config.RequiredVariables {
		if !foundVariables[reqVar.Name] {
			errors = append(errors, ValidationError{
				Message: fmt.Sprintf("Required variable '%s' not found - needed by tf-repo automation. See %s for the expected structure", reqVar.Name, TerraformRepoTemplateURL),
			})
		}
	}

	for _, reqProvider := range config.RequiredProviders {
		if !foundProviders[reqProvider.LocalName] {
			errors = append(errors, ValidationError{
				Message: fmt.Sprintf("Required provider '%s' not found in required_providers block - source should be '%s'. See %s for the expected structure", reqProvider.LocalName, reqProvider.Source, TerraformRepoTemplateURL),
			})
		}
	}

	return errors
}
