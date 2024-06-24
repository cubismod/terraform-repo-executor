// Package vaultutil includes helper functions for communicating with Vault
package vaultutil

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-exec/tfexec"
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

// WriteOutputs takes any output values from a Terraform apply and then writes them into Vault
func WriteOutputs(client *vault.Client, secretInfo VaultSecret, data map[string]tfexec.OutputMeta, kvVersion string) error {
	log.Printf("Writing Output values from Terraform Apply to %s in Vault", secretInfo.Path)
	secretData := make(VaultKvData)

	for k, v := range data {
		var value interface{}
		err := json.Unmarshal(v.Value, &value)
		if err != nil {
			return err
		}
		secretData[k] = value
	}

	err := WriteVaultSecret(client, secretInfo, secretData, kvVersion)
	if err != nil {
		return err
	}
	return nil
}

// converts a secret path to KvV2 formatting
// https://developer.hashicorp.com/vault/api-docs/secret/kv/kv-v2#create-update-secret
// api calls to vault kv v2 secret engines expect 'data' path between root (secret engine name)
// and remaining path
func convertPathKvV2(path string) (string, error) {
	sliced := strings.SplitN(path, "/", 2)
	if len(sliced) < 2 {
		return "", fmt.Errorf("invalid vault path: %s", path)
	}
	return fmt.Sprintf("%s/data/%s", sliced[0], sliced[1]), nil
}

// splits the path into the mount (1st return) and secret path (2nd return)
func splitVaultPath(path string) (string, string, error) {
	before, after, found := strings.Cut(path, "/")

	if !found {
		return "", "", fmt.Errorf("invalid vault path: %s", path)
	}
	return before, after, nil
}

// WriteVaultSecret writes a map of KV pairs to Vault at the specified path
func WriteVaultSecret(client *vault.Client, secretInfo VaultSecret, data map[string]interface{}, kvVersion string) error {
	mount, path, err := splitVaultPath(secretInfo.Path)
	if err != nil {
		return err
	}
	if kvVersion == KvV2 {
		_, err = client.KVv2(mount).Put(context.Background(), path, data)
		return err
	}
	return client.KVv1(mount).Put(context.Background(), path, data)
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
		path, err := convertPathKvV2(secretInfo.Path)
		if err != nil {
			return nil, err
		}
		// version is optional in config yaml
		// default behavior when omitted will be to use latest
		var rawSecret *vault.Secret
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
