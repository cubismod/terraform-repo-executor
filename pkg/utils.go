package pkg

import (
	"bytes"
	"errors"
	"os"
	"os/exec"

	"gopkg.in/yaml.v2"
)

func processConfig(cfgPath string) (*TfRepo, error) {
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}

	var cfg TfRepo
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
