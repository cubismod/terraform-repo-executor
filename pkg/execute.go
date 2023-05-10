package pkg

import (
	"fmt"

	vault "github.com/hashicorp/vault/api"
)

type TfRepo struct {
	Name   string      `yaml:"name"`
	Url    string      `yaml:"repository"`
	Path   string      `yaml:"project_path"`
	Ref    string      `yaml:"ref"`
	Delete bool        `yaml:"delete"`
	DryRun bool        `yaml:"dry_run"`
	Secret VaultSecret `yaml:"secret"`
}

type VaultSecret struct {
	Path    string `yaml:"path"`
	Version int    `yaml:"version"`
}

type Executor struct {
	TfRepoCfg     *TfRepo
	vaultClient   *vault.Client
	glUsername    string
	glToken       string
	workdir       string
	vaultAddr     string
	vaultRoleId   string
	vaultSecretId string
}

func Run(cfgPath,
	workdir,
	glUsername,
	glToken,
	vaultAddr,
	roleId,
	secretId string) error {

	// clear working directory upon exit
	defer executeCommand("/", "rm", []string{"-rf", workdir})

	_, err := executeCommand("/", "mkdir", []string{workdir})
	if err != nil {
		return err
	}

	targetRepo, err := processConfig(cfgPath)
	if err != nil {
		return err
	}

	e := &Executor{
		TfRepoCfg:     targetRepo,
		vaultClient:   initVaultClient(vaultAddr, roleId, secretId),
		glUsername:    glUsername,
		glToken:       glToken,
		workdir:       workdir,
		vaultAddr:     vaultAddr,
		vaultRoleId:   roleId,
		vaultSecretId: secretId,
	}

	err = e.execute()
	if err != nil {
		return err
	}
	return nil
}

type errObj struct {
	msg  string
	name string
}

// performs all repo-specific operations
func (e *Executor) execute() error {
	err := e.cloneRepo()
	if err != nil {
		return err
	}

	TfBackend, err := e.getVaultSecrets()
	if err != nil {
		return err
	}

	TfBackend.Key = fmt.Sprintf("%s-tf-repo.tfstate", e.TfRepoCfg.Name)
	err = e.generateBackendFile(TfBackend)
	if err != nil {
		return err
	}

	err = e.generateTfVarsFile(TfBackend)
	if err != nil {
		return err
	}

	err = e.processTfPlan()
	if err != nil {
		return err
	}

	return nil
}
