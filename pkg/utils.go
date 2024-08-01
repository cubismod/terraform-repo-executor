package pkg

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"gopkg.in/yaml.v2"
)

// FolderPerm is 0770 in chmod
const FolderPerm = 0770

func processConfig(cfgPath string) (*Input, error) {
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}

	var cfg Input
	// internally yaml.Unmarshal will use json.Unmarshal if it detects json format
	err = yaml.Unmarshal(raw, &cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// generic function for executing commands on host
func executeCommand(dir, command string, args []string) (string, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.String(), errors.New(stderr.String())
	}
	return stdout.String(), nil
}

// GetCABundle retrieves custom certificate authorities needed to process a Git repo and returns a byte slice or
// an empty slice if no CAs are provided
func GetCABundle() []byte {
	bundlePath := os.Getenv("GIT_SSL_CAINFO")
	if bundlePath != "" {
		bundle, err := os.ReadFile(bundlePath)
		if err != nil {
			return nil
		}

		return bundle
	}
	return nil
}

func (r Repo) cloneRepo(workdir string) error {
	// go-git doesn't create a new directory in the cloned dir so we have to create one ourselves
	clonedDir := fmt.Sprintf("%s/%s", workdir, r.Name)
	err := os.Mkdir(clonedDir, FolderPerm)
	if err != nil {
		return err
	}

	repo, err := git.PlainClone(clonedDir, false, &git.CloneOptions{
		URL:      r.URL,
		CABundle: GetCABundle(),
	})
	if err != nil {
		return err
	}

	wt, err := repo.Worktree()
	if err != nil {
		return err
	}

	err = wt.Checkout(&git.CheckoutOptions{
		Hash: plumbing.NewHash(r.Ref),
	})
	if err != nil {
		return err
	}
	return nil
}

// MaskSensitiveStateValues redacts any Vault secrets in a Terraform human-readable state file
func MaskSensitiveStateValues(src string) string {
	re := regexp.MustCompile(`(?sU)(data "vault_generic_secret".+\n})`)
	return re.ReplaceAllString(src, "[REDACTED VAULT SECRET]")
}
