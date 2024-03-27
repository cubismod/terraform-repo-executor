// Package vaultutil includes helper functions for communicating with Vault
package vaultutil

import (
	"fmt"
	"strings"

	vault "github.com/hashicorp/vault/api"
)

// VaultSecret contains information on where to find a secret in Vault
type VaultSecret struct {
	Path    string `yaml:"path" json:"path"`
	Version int    `yaml:"version" json:"version"`
}

// VaultKvData includes KV pairings of Vault secrets
type VaultKvData map[string]interface{}

// which KV engine a particular vault mount is using
const (
	KvV1 = "KV_V1"
	KvV2 = "KV_V2"
)

// InitVaultClient sets up a Vault client that logs in using AppRole credentials
func InitVaultClient(addr, roleID, secretID string) (*vault.Client, error) {
	cfg := &vault.Config{
		Address: addr,
	}
	client, err := vault.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	// authenticate using approle
	data := map[string]interface{}{
		"role_id":   roleID,
		"secret_id": secretID,
	}
	secret, err := client.Logical().Write("auth/approle/login", data)
	if err != nil {
		return nil, err
	}
	if secret == nil || secret.Auth == nil {
		return nil, err
	}

	client.SetToken(secret.Auth.ClientToken)

	return client, nil
}

// GetVaultTfSecret retrieves the contents of a secret in Vault
func GetVaultTfSecret(client *vault.Client, secretInfo VaultSecret, kvVersion string) (VaultKvData, error) {
	var secret VaultKvData

	switch kvVersion {
	case KvV1:
		rawSecret, err := client.Logical().Read(secretInfo.Path)
		if err != nil {
			return nil, err
		}
		if rawSecret == nil {
			return nil, fmt.Errorf("no secret found at specified path: %s", secretInfo.Path)
		}
		if len(rawSecret.Data) == 0 {
			return nil, fmt.Errorf("no key-values stored within secret at path: %s", secretInfo.Path)
		}
		secret = rawSecret.Data
	case KvV2:
		// api calls to vault kv v2 secret engines expect 'data' path between root (secret engine name)
		// and remaining path
		sliced := strings.SplitN(secretInfo.Path, "/", 2)
		if len(sliced) < 2 {
			return nil, fmt.Errorf("invalid vault path: %s", secretInfo.Path)
		}
		path := fmt.Sprintf("%s/data/%s", sliced[0], sliced[1])
		// version is optional in config yaml
		// default behavior when omitted will be to use latest
		var rawSecret *vault.Secret
		var err error
		if secretInfo.Version != 0 {
			rawSecret, err = client.Logical().ReadWithData(path, map[string][]string{
				"version": {fmt.Sprintf("%d", secretInfo.Version)},
			})
		} else {
			rawSecret, err = client.Logical().Read(path)
		}
		if err != nil {
			return nil, err
		}
		if rawSecret == nil {
			return nil, fmt.Errorf("no secret found at specified path: %s", secretInfo.Path)
		}
		if len(rawSecret.Data) == 0 {
			return nil, fmt.Errorf("no key-values stored within secret at path: %s", secretInfo.Path)
		}
		var ok bool
		secret, ok = rawSecret.Data["data"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("failed to process data for secret at path: %s", secretInfo.Path)
		}
	default:
		return nil, fmt.Errorf("invalid vault kv engine version specified: %s", kvVersion)
	}

	return secret, nil
}
