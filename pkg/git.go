package pkg

import (
	"fmt"
	"os"
)

func (e *Executor) cloneRepo() error {
	// optionally trust internal git server cert
	caPath := os.Getenv("INTERNAL_GIT_CA_PATH")

	if caPath != "" {
		args := []string{"-c", fmt.Sprintf(
			"git config http.sslCAInfo %s",
			caPath,
		)}

		_, err := executeCommand(e.workdir, "/bin/sh", args)
		if err != nil {
			return err
		}
	}

	args := []string{"-c", fmt.Sprintf(
		// clone repo with specified name and checkout specified ref
		"git clone %s %s && cd %s && git checkout %s",
		e.TfRepoCfg.Url,
		e.TfRepoCfg.Name,
		e.TfRepoCfg.Name,
		e.TfRepoCfg.Ref,
	)}
	_, err := executeCommand(e.workdir, "/bin/sh", args)
	if err != nil {
		return err
	}
	return nil
}
