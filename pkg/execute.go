// Package pkg contains logic for executing Terraform actions
package pkg

import (
	"fmt"
	"log"
	"os"

	"github.com/app-sre/terraform-repo-executor/pkg/vaultutil"
	vault "github.com/hashicorp/vault/api"
)

// Input holds YAML/JSON loaded from CONFIG_FILE and is passed from Qontract Reconcile
type Input struct {
	DryRun bool   `yaml:"dry_run" json:"dry_run"`
	Repos  []Repo `yaml:"repos" json:"repos"`
}

// Repo represents an individual Terraform Repo
type Repo struct {
	Name        string                `yaml:"name" json:"name"`
	URL         string                `yaml:"repository" json:"repository"`
	Path        string                `yaml:"project_path" json:"project_path"`
	Ref         string                `yaml:"ref" json:"ref"`
	Delete      bool                  `yaml:"delete" json:"delete"`
	AWSCreds    vaultutil.VaultSecret `yaml:"aws_creds" json:"aws_creds"`
	Bucket      string                `yaml:"bucket,omitempty" json:"bucket,omitempty"`
	Region      string                `yaml:"region,omitempty" json:"region,omitempty"`
	BucketPath  string                `yaml:"bucket_path,omitempty" json:"bucket_path,omitempty"`
	RequireFips bool                  `yaml:"require_fips" json:"require_fips"`
	TfVersion   string                `yaml:"tf_version" json:"tf_version"`
	TfVariables TfVariables           `yaml:"variables,omitempty" json:"variables,omitempty"`
}

// TfVariables are references to Vault paths used for reading/writing inputs and outputs
type TfVariables struct {
	Inputs  vaultutil.VaultSecret `yaml:"inputs" json:"inputs"`
	Outputs vaultutil.VaultSecret `yaml:"outputs" json:"outputs"`
}

// Executor includes required secrets and variables to perform a tf repo executor run
type Executor struct {
	workdir          string
	vaultAddr        string
	vaultRoleID      string
	vaultSecretID    string
	vaultTfKvVersion string
}

// Run is responsible for the full lifecycle of creating/updating/deleting a Terraform repo.
// Including loading config, secrets from vault, creation and cleanup of temp directories and the actual Terraform operations
func Run(cfgPath,
	workdir,
	vaultAddr,
	roleID,
	secretID,
	kvVersion string) error {

	// clear working directory upon exit
	defer os.RemoveAll(workdir)

	err := os.Mkdir(workdir, 0770)
	if err != nil {
		return err
	}

	cfg, err := processConfig(cfgPath)
	if err != nil {
		return err
	}

	vaultClient, err := vaultutil.InitVaultClient(vaultAddr, roleID, secretID)
	if err != nil {
		return err
	}

	// vault creds are stored for later usage when generating tfvars for vault provider
	e := &Executor{
		workdir:          workdir,
		vaultAddr:        vaultAddr,
		vaultRoleID:      roleID,
		vaultSecretID:    secretID,
		vaultTfKvVersion: kvVersion,
	}

	errCounter := 0
	for _, repo := range cfg.Repos {
		err = e.execute(repo, vaultClient, cfg.DryRun)
		if err != nil {
			log.Printf("Error executing terraform operations for: %s\n", repo.Name)
			log.Println(err)
			errCounter++
		}
	}

	if errCounter > 0 {
		return fmt.Errorf("errors encountered within %d/%d targets", errCounter, len(cfg.Repos))
	}
	return nil
}

// performs all repo-specific operations
func (e *Executor) execute(repo Repo, vaultClient *vault.Client, dryRun bool) error {
	err := repo.cloneRepo(e.workdir)
	if err != nil {
		return err
	}

	secret, err := vaultutil.GetVaultTfSecret(vaultClient, repo.AWSCreds, e.vaultTfKvVersion)
	if err != nil {
		return err
	}

	backendCreds, err := extractTfCreds(secret, repo)
	if err != nil {
		return err
	}

	if len(repo.BucketPath) > 0 {
		backendCreds.Key = fmt.Sprintf("%s/%s-tf-repo.tfstate", repo.BucketPath, repo.Name)
	} else {
		backendCreds.Key = fmt.Sprintf("%s-tf-repo.tfstate", repo.Name)
	}
	err = e.generateBackendFile(backendCreds, repo)
	if err != nil {
		return err
	}

	err = e.generateCredVarsFile(backendCreds, repo)
	if err != nil {
		return err
	}

	if repo.TfVariables.Inputs.Path != "" {
		// extract kv pairs from vault for inputs and write them to a file for terraform usage
		inputSecret, err := vaultutil.GetVaultTfSecret(vaultClient, repo.TfVariables.Inputs, e.vaultTfKvVersion)
		if err != nil {
			return err
		}

		err = e.generateInputVarsFile(inputSecret, repo)
		if err != nil {
			return err
		}
	}

	output, err := e.processTfPlan(repo, dryRun)
	if err != nil {
		return err
	}

	if len(output) > 0 && repo.TfVariables.Outputs.Path != "" {
		err = vaultutil.WriteOutputs(vaultClient, repo.TfVariables.Outputs, output)
		if err != nil {
			return err
		}
	}

	return nil
}
