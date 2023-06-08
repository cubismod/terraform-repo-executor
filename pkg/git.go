package pkg

import (
	"fmt"
)

func (e *Executor) cloneRepo() error {
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
