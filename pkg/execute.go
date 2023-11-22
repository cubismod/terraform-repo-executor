package pkg

import (
	"fmt"
	"log"

	"github.com/app-sre/terraform-repo-executor/pkg/vaultutil"
	vault "github.com/hashicorp/vault/api"
)

type TfRepo struct {
	DryRun bool   `yaml:"dry_run" json:"dry_run"`
	Repos  []Repo `yaml:"repos" json:"repos"`
}

type Repo struct {
	Name        string                `yaml:"name" json:"name"`
	Url         string                `yaml:"repository" json:"repository"`
	Path        string                `yaml:"project_path" json:"project_path"`
	Ref         string                `yaml:"ref" json:"ref"`
	Delete      bool                  `yaml:"delete" json:"delete"`
	Secret      vaultutil.VaultSecret `yaml:"secret" json:"secret"`
	Bucket      string                `yaml:"bucket,omitempty" json:"bucket,omitempty"`
	Region      string                `yaml:"region,omitempty" json:"region,omitempty"`
	BucketPath  string                `yaml:"bucket_path,omitempty" json:"bucket_path,omitempty"`
	RequireFips bool                  `yaml:"require_fips" json:"require_fips"`
}

type Executor struct {
	vaultClient      *vault.Client
	workdir          string
	vaultAddr        string
	vaultRoleId      string
	vaultSecretId    string
	vaultTfKvVersion string
}

func Run(cfgPath,
	workdir,
	vaultAddr,
	roleId,
	secretId,
	kvVersion string) error {

	// clear working directory upon exit
	defer executeCommand("/", "rm", []string{"-rf", workdir})

	_, err := executeCommand("/", "mkdir", []string{workdir})
	if err != nil {
		return err
	}

	cfg, err := processConfig(cfgPath)
	if err != nil {
		return err
	}

	vaultClient, err := vaultutil.InitVaultClient(vaultAddr, roleId, secretId)
	if err != nil {
		return err
	}

	// vault creds are stored for later usage when generating tfvars for vault provider
	e := &Executor{
		workdir:          workdir,
		vaultAddr:        vaultAddr,
		vaultRoleId:      roleId,
		vaultSecretId:    secretId,
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
		return fmt.Errorf("Errors encountered within %d/%d targets", errCounter, len(cfg.Repos))
	}
	return nil
}

type errObj struct {
	msg  string
	name string
}

// performs all repo-specific operations
func (e *Executor) execute(repo Repo, vaultClient *vault.Client, dryRun bool) error {
	err := repo.cloneRepo(e.workdir)
	if err != nil {
		return err
	}

	secret, err := vaultutil.GetVaultTfSecret(vaultClient, repo.Secret, e.vaultTfKvVersion)
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

	err = e.generateTfVarsFile(backendCreds, repo)
	if err != nil {
		return err
	}

	err = e.processTfPlan(repo, dryRun)
	if err != nil {
		return err
	}

	return nil
}
