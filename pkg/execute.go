package pkg

import (
	"fmt"
	"log"
	"sync"

	vault "github.com/hashicorp/vault/api"
)

type Executor struct {
	TfRepoCfg     *TfRepo
	initSync      sync.Mutex
	vaultClient   *vault.Client
	glUsername    string
	glToken       string
	workdir       string
	vaultAddr     string
	vaultRoleId   string
	vaultSecretId string
}

type TfRepo struct {
	DryRun  bool     `yaml:"dry_run"`
	Targets []Target `yaml:"repos"`
}

type Target struct {
	Name    string  `yaml:"name"`
	Url     string  `yaml:"repository"`
	Ref     string  `yaml:"ref"`
	Path    string  `yaml:"project_path"`
	Delete  bool    `yaml:"delete"`
	Account Account `yaml:"account"`
}

type Account struct {
	Name   string      `yaml:"name"`
	UID    string      `yaml:"uid"`
	Secret VaultSecret `yaml:"secret"`
}

type VaultSecret struct {
	Path    string `yaml:"path"`
	Version int    `yaml:"version"`
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

	tfRepo, err := processConfig(cfgPath)
	if err != nil {
		return err
	}

	e := &Executor{
		TfRepoCfg:     tfRepo,
		vaultClient:   initVaultClient(vaultAddr, roleId, secretId),
		glUsername:    glUsername,
		glToken:       glToken,
		workdir:       workdir,
		vaultAddr:     vaultAddr,
		vaultRoleId:   roleId,
		vaultSecretId: secretId,
	}

	var wg sync.WaitGroup
	errCh := make(chan errObj)

	// concurrently perform git, vault, and tf ops for each repo defined in config
	wg.Add(len(tfRepo.Targets))
	for _, repo := range tfRepo.Targets {
		go e.execute(errCh, &wg, repo)
	}

	go func() {
		wg.Wait()
		close(errCh)
	}()

	for err := range errCh {
		log.Printf("Error performing reconcile for: %s\nERROR: %s", err.name, err.msg)
	}
	return nil
}

type errObj struct {
	msg  string
	name string
}

// goroutine function to perform all repo-specific operations
func (e *Executor) execute(ch chan<- errObj, wg *sync.WaitGroup, repo Target) {
	defer wg.Done()
	err := e.cloneRepo(repo)
	if err != nil {
		ch <- errObj{
			msg:  err.Error(),
			name: repo.Name,
		}
		return
	}

	TfBackend, err := e.getVaultSecrets(repo.Account.Secret.Path, repo.Account.Secret.Version)
	if err != nil {
		ch <- errObj{
			msg:  err.Error(),
			name: repo.Name,
		}
		return
	}

	TfBackend.Key = fmt.Sprintf("%s-tf-repo.tfstate", repo.Name)
	err = e.generateBackendFile(repo, TfBackend)
	if err != nil {
		ch <- errObj{
			msg:  err.Error(),
			name: repo.Name,
		}
		return
	}

	err = e.generateTfVarsFile(repo, TfBackend)
	if err != nil {
		ch <- errObj{
			msg:  err.Error(),
			name: repo.Name,
		}
		return
	}

	err = e.processTfPlan(e.TfRepoCfg.DryRun, repo)
	if err != nil {
		ch <- errObj{
			msg:  err.Error(),
			name: repo.Name,
		}
	}
}
