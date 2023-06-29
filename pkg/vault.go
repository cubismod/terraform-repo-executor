package pkg

import (
	"fmt"
	"strings"

	vault "github.com/hashicorp/vault/api"
)

const (
	KV_V1 = "KV_V1"
	KV_V2 = "KV_V2"
)

func initVaultClient(addr, roleId, secretId string) (*vault.Client, error) {
	cfg := &vault.Config{
		Address: addr,
	}
	client, err := vault.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	// authenticate using approle
	data := map[string]interface{}{
		"role_id":   roleId,
		"secret_id": secretId,
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

// standardized AppSRE terraform secret keys
const (
	AWS_ACCESS_KEY_ID     = "aws_access_key_id"
	AWS_SECRET_ACCESS_KEY = "aws_secret_access_key"
	AWS_REGION            = "region"
	AWS_BUCKET            = "bucket"
)

type VaultKvData map[string]interface{}

// returns secret data from a vault kv v1 or v2 secret engine path
func getVaultTfSecret(client *vault.Client, secretInfo VaultSecret, kvVersion string) (TfCreds, error) {
	var secret VaultKvData

	switch kvVersion {
	case KV_V1:
		rawSecret, err := client.Logical().Read(secretInfo.Path)
		if err != nil {
			return TfCreds{}, err
		}
		if rawSecret == nil {
			return TfCreds{}, fmt.Errorf("No secret found at specified path: %s", secretInfo.Path)
		}
		if len(rawSecret.Data) == 0 {
			return TfCreds{}, fmt.Errorf("No key-values stored within secret at path: %s", secretInfo.Path)
		}
		secret = rawSecret.Data
	case KV_V2:
		// api calls to vault kv v2 secret engines expect 'data' path between root (secret engine name)
		// and remaining path
		sliced := strings.SplitN(secretInfo.Path, "/", 2)
		if len(sliced) < 2 {
			return TfCreds{}, fmt.Errorf("Invalid vault path: %s", secretInfo.Path)
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
			return TfCreds{}, err
		}
		if rawSecret == nil {
			return TfCreds{}, fmt.Errorf("No secret found at specified path: %s", secretInfo.Path)
		}
		if len(rawSecret.Data) == 0 {
			return TfCreds{}, fmt.Errorf("No key-values stored within secret at path: %s", secretInfo.Path)
		}
		var ok bool
		secret, ok = rawSecret.Data["data"].(map[string]interface{})
		if !ok {
			return TfCreds{}, fmt.Errorf("Failed to process data for secret at path: %s", secretInfo.Path)
		}
	default:
		return TfCreds{}, fmt.Errorf("Invalid vault kv engine version specified: %s", kvVersion)
	}

	for _, key := range []string{AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_BUCKET, AWS_REGION} {
		if secret[key] == nil {
			return TfCreds{}, fmt.Errorf("Failed to retrieve %s for secret at path: %s", key, secretInfo.Path)
		}
	}
	return TfCreds{
		AccessKey: secret[AWS_ACCESS_KEY_ID].(string),
		SecretKey: secret[AWS_SECRET_ACCESS_KEY].(string),
		Bucket:    secret[AWS_BUCKET].(string),
		Region:    secret[AWS_REGION].(string),
	}, nil
}
