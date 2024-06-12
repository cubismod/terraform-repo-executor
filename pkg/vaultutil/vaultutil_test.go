package vaultutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-exec/tfexec"
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

func TestWriteVaultOutputs(t *testing.T) {
	type payload struct {
		VPCID string `json:"vpc_id"`
	}

	type data struct {
		Data payload `json:"data"`
	}

	planOutput := map[string]tfexec.OutputMeta{
		"vpc_id": {
			Sensitive: false,
			Type:      json.RawMessage(`string`),
			Value:     json.RawMessage(`"vpc-22fd8eb8"`),
		},
	}

	expectedBody := data{
		Data: payload{
			VPCID: "vpc-22fd8eb8",
		},
	}

	vaultMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var decoded data
		err := json.NewDecoder(r.Body).Decode(&decoded)

		assert.Nil(t, err)

		assert.Equal(t, "/v1/terraform/data/stage/outputs", r.URL.Path)
		assert.Equal(t, expectedBody, decoded)
		// function doesn't care about the returned info just that there is no error
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"hello": "world"})
	}))

	defer vaultMock.Close()
	client, _ := vault.NewClient(&vault.Config{
		Address: vaultMock.URL,
	})

	err := WriteOutputs(client, VaultSecret{
		Path: "terraform/stage/outputs",
	}, planOutput, KvV2)

	assert.Nil(t, err)
}
