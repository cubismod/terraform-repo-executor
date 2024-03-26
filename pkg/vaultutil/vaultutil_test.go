package vaultutil

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	vault "github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/assert"
)

func TestInitVaultClient(t *testing.T) {
	mockedToken := "65b74ffd-842c-fd43-1386-f7d7006e520a"
	vaultMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "auth/approle/login")
		body, err := io.ReadAll(r.Body)
		assert.Nil(t, err)
		assert.Equal(t, `{"role_id":"foo","secret_id":"bar"}`, string(body))
		fmt.Fprintf(w, `{"auth": {"client_token": "%s"}}`, mockedToken)
	}))
	defer vaultMock.Close()

	roleID := "foo"
	secretID := "bar"
	client, err := InitVaultClient(vaultMock.URL, roleID, secretID)
	assert.Nil(t, err)
	assert.Equal(t, mockedToken, client.Token())
}

func TestGetVaultTfSecretV2(t *testing.T) {
	mockedData := `{
		"data": {
			"data": {
			  	"aws_access_key_id": "foo",
				"aws_secret_access_key": "bar",
				"region": "weast",
				"bucket": "head"
			},
			"metadata": {}
		}
}`
	vaultMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "v1/terraform/data/stage")
		assert.Equal(t, "3", r.URL.Query().Get("version"))
		fmt.Fprint(w, mockedData)
	}))
	defer vaultMock.Close()

	client, _ := vault.NewClient(&vault.Config{
		Address: vaultMock.URL,
	})

	actual, err := GetVaultTfSecret(client, VaultSecret{
		Path:    "terraform/stage",
		Version: 3,
	}, KvV2)
	assert.Nil(t, err)

	expected := VaultKvData{
		"aws_access_key_id":     "foo",
		"aws_secret_access_key": "bar",
		"region":                "weast",
		"bucket":                "head",
	}

	assert.Equal(t, expected, actual)
}

func TestGetVaultTfSecretV1(t *testing.T) {
	mockedData := `{
		"data": {
		  	"aws_access_key_id": "foo",
			"aws_secret_access_key": "bar",
			"region": "weast",
			"bucket": "head"
		}
}`
	vaultMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "v1/terraform/stage")
		fmt.Fprint(w, mockedData)
	}))
	defer vaultMock.Close()

	client, _ := vault.NewClient(&vault.Config{
		Address: vaultMock.URL,
	})

	actual, err := GetVaultTfSecret(client, VaultSecret{
		Path:    "terraform/stage",
		Version: 1,
	}, KvV1)
	assert.Nil(t, err)

	expected := VaultKvData{
		"aws_access_key_id":     "foo",
		"aws_secret_access_key": "bar",
		"region":                "weast",
		"bucket":                "head",
	}

	assert.Equal(t, expected, actual)
}
