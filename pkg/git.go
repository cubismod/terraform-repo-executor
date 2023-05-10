package pkg

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func (e *Executor) cloneRepo() error {
	authUrl, err := e.getAuthUrl(e.TfRepoCfg.Url)
	if err != nil {
		return err
	}

	args := []string{"-c", fmt.Sprintf(
		// clone repo with specified name and checkout specified ref
		"git clone %s %s && cd %s && git checkout %s",
		authUrl,
		e.TfRepoCfg.Name,
		e.TfRepoCfg.Name,
		e.TfRepoCfg.Ref,
	)}
	_, err = executeCommand(e.workdir, "/bin/sh", args)
	if err != nil {
		return errors.New(strings.ReplaceAll(err.Error(), e.glToken, "[REDACTED]"))
	}
	return nil
}

func (e *Executor) getAuthUrl(u string) (string, error) {
	parsedUrl, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s://%s:%s@%s%s.git",
		parsedUrl.Scheme,
		e.glUsername,
		e.glToken,
		parsedUrl.Host,
		parsedUrl.Path,
	), nil
}
