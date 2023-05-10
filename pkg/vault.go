package pkg

import (
	"fmt"
	"log"
	"strings"

	vault "github.com/hashicorp/vault/api"
)

func initVaultClient(addr, roleId, secretId string) *vault.Client {
	cfg := &vault.Config{
		Address: addr,
	}
	client, err := vault.NewClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create vault client %v", err)
	}

	// authenticate using approle
	data := map[string]interface{}{
		"role_id":   roleId,
		"secret_id": secretId,
	}
	secret, err := client.Logical().Write("auth/approle/login", data)
	if err != nil {
		log.Fatalf("Failed to log in with AppRole: %v", err)
	}
	if secret == nil || secret.Auth == nil {
		log.Fatal("No authentication data returned")
	}

	client.SetToken(secret.Auth.ClientToken)

	return client
}

// standardized AppSRE terraform secret keys
const (
	AWS_ACCESS_KEY_ID     = "aws_access_key_id"
	AWS_SECRET_ACCESS_KEY = "aws_secret_access_key"
	AWS_REGION            = "region"
	AWS_BUCKET            = "bucket"
)

// expects appSRE standardized terraform secret keys to exist
// NOTE: this logic is specific to a KV V2 secret engine
func (e *Executor) getVaultSecrets() (TfBackend, error) {
	// api calls to vault kv v2 secret engines expect 'data' path between root (secret engine name)
	// and remaining path
	sliced := strings.SplitN(e.TfRepoCfg.Secret.Path, "/", 2)
	if len(sliced) < 2 {
		return TfBackend{}, fmt.Errorf("Invalid vault path: %s", e.TfRepoCfg.Secret.Path)
	}
	formattedPath := fmt.Sprintf("%s/data/%s", sliced[0], sliced[1])

	var secret *vault.Secret
	var err error
	// version is optional in config yaml
	// default behavior when omitted will be to use latest
	if e.TfRepoCfg.Secret.Version != 0 {
		secret, err = e.vaultClient.Logical().ReadWithData(formattedPath, map[string][]string{
			"version": {fmt.Sprintf("%d", e.TfRepoCfg.Secret.Version)},
		})
	} else {
		secret, err = e.vaultClient.Logical().Read(formattedPath)
	}

	if err != nil {
		return TfBackend{}, err
	}
	if secret == nil {
		return TfBackend{}, fmt.Errorf("No secret found at specified path: %s", e.TfRepoCfg.Secret.Path)
	}
	if len(secret.Data) == 0 {
		return TfBackend{}, fmt.Errorf("No key-values stored within secret at path: %s", e.TfRepoCfg.Secret.Path)
	}
	mappedSecret, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return TfBackend{}, fmt.Errorf("Failed to process data for secret at path: %s", e.TfRepoCfg.Secret.Path)
	}

	for _, key := range []string{AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_BUCKET, AWS_REGION} {
		if mappedSecret[key] == nil {
			return TfBackend{}, fmt.Errorf("Failed to retrieve %s for secret at path: %s", key, e.TfRepoCfg.Secret.Path)
		}
	}

	return TfBackend{
		AccessKey: mappedSecret[AWS_ACCESS_KEY_ID].(string),
		SecretKey: mappedSecret[AWS_SECRET_ACCESS_KEY].(string),
		Bucket:    mappedSecret[AWS_BUCKET].(string),
		Region:    mappedSecret[AWS_REGION].(string),
	}, nil
}
