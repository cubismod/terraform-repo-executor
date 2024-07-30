// Package pkg contains logic for executing Terraform actions
package pkg

import (
	"fmt"
	"log"
	"os"
	"time"

	_ "embed"

	"github.com/app-sre/terraform-repo-executor/pkg/vaultutil"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
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
	workdir        string
	vaultAddr      string
	vaultRoleID    string
	vaultSecretID  string
	gitlabLogRepo  string
	gitlabUsername string
	gitlabToken    string
	gitEmail       string
	mountVersions  map[string]string
}

// StateVars are used to render the raw statefile in markdown
type StateVars struct {
	RepoName string
	RepoURL  string
	RepoSHA  string
	State    string
}

//go:embed templates/show.tmpl
var tmplData string

// Run is responsible for the full lifecycle of creating/updating/deleting a Terraform repo.
// Including loading config, secrets from vault, creation and cleanup of temp directories and the actual Terraform operations
func Run(cfgPath,
	workdir,
	vaultAddr,
	roleID,
	secretID,
	gitlabLogRepo,
	gitlabUsername,
	gitlabToken,
	gitEmail string) error {

	cfg, err := processConfig(cfgPath)
	if err != nil {
		return err
	}

	vaultClient, err := vaultutil.InitVaultClient(vaultAddr, roleID, secretID)
	if err != nil {
		return err
	}

	mountVersions, err := vaultutil.GetMountVersions(vaultClient)
	if err != nil {
		return fmt.Errorf("unable to retrieve information about mounted secret engines, please ensure that tf-repo AppRole has access to /sys/mounts. Further info: %s", err)
	}

	// vault creds are stored for later usage when generating tfvars for vault provider
	e := &Executor{
		workdir:        workdir,
		vaultAddr:      vaultAddr,
		vaultRoleID:    roleID,
		vaultSecretID:  secretID,
		gitlabLogRepo:  gitlabLogRepo,
		gitlabUsername: gitlabUsername,
		gitlabToken:    gitlabToken,
		gitEmail:       gitEmail,
		mountVersions:  mountVersions,
	}

	errCounter := 0
	for _, repo := range cfg.Repos {
		// there needs to be a clean working directory for each repository
		err := os.Mkdir(workdir, FolderPerm)
		if err != nil {
			return err
		}

		err = e.execute(repo, vaultClient, cfg.DryRun)
		if err != nil {
			log.Printf("Error executing terraform operations for: %s\n", repo.Name)
			log.Println(err)
			errCounter++
		}

		err = os.RemoveAll(workdir)
		if err != nil {
			return err
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

	secret, err := vaultutil.GetVaultTfSecret(vaultClient, repo.AWSCreds, e.mountVersions)
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
		inputSecret, err := vaultutil.GetVaultTfSecret(vaultClient, repo.TfVariables.Inputs, e.mountVersions)
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

	if output != nil && repo.TfVariables.Outputs.Path != "" {
		err = vaultutil.WriteOutputs(vaultClient, repo.TfVariables.Outputs, output, e.mountVersions)
		if err != nil {
			return err
		}
	}

	return nil
}

// clones the output repo, writes the raw state to a file, commits and pushes that to GitLab
func (e *Executor) commitAndPushState(repo Repo, state string) error {
	gitAuth := &http.BasicAuth{
		Username: e.gitlabUsername,
		Password: e.gitlabToken,
	}

	tmpdir, err := os.MkdirTemp("", "tf-repo-state")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)

	gitRepo, err := git.PlainClone(tmpdir, false, &git.CloneOptions{
		URL:      e.gitlabLogRepo,
		Auth:     gitAuth,
		CABundle: GetCABundle(),
	})
	if err != nil {
		return fmt.Errorf("could not clone repo: '%s'", err)
	}

	// prepare to template a markdown file
	stateVars := &StateVars{
		RepoName: repo.Name,
		RepoURL:  repo.URL,
		RepoSHA:  repo.Ref,
		State:    state,
	}

	err = WriteTemplate(*stateVars, tmplData, fmt.Sprintf("%s/%s.md", tmpdir, repo.Name))
	if err != nil {
		return fmt.Errorf("could not template markdown: '%s'", err)
	}

	wt, err := gitRepo.Worktree()
	if err != nil {
		return fmt.Errorf("could not retrieve git worktree: '%s'", err)
	}

	st, err := wt.Status()
	if err != nil {
		return fmt.Errorf("could not retrieve worktree status: '%s'", err)
	}

	_, err = wt.Add(fmt.Sprintf("%s.md", repo.Name))
	if err != nil {
		return fmt.Errorf("could not perform git add: '%s'", err)
	}

	if !st.IsClean() {
		// no need to commit changes if nothing changed
		_, err = wt.Commit(fmt.Sprintf("%s: %s", repo.Name, time.Now().Format(time.RFC3339)), &git.CommitOptions{
			Author: &object.Signature{
				Name:  e.gitlabUsername,
				Email: e.gitEmail,
				When:  time.Now(),
			},
		})
		if err != nil {
			return fmt.Errorf("could not perform git commit: '%s'", err)
		}

		err = gitRepo.Push(&git.PushOptions{
			RemoteName: "origin",
			CABundle:   GetCABundle(),
			Auth:       gitAuth,
		})
		if err != nil {
			return fmt.Errorf("could not push git commit to remote: '%s'", err)
		}
	}
	return nil
}
