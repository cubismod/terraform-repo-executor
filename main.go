package main

import (
	"log"
	"os"

	"github.com/app-sre/terraform-repo-executor/pkg"
	"github.com/app-sre/terraform-repo-executor/pkg/vaultutil"
)

const (
	CONFIG_FILE         = "CONFIG_FILE"
	VAULT_ADDR          = "VAULT_ADDR"
	VAULT_ROLE_ID       = "VAULT_ROLE_ID"
	VAULT_SECRET_ID     = "VAULT_SECRET_ID"
	VAULT_TF_KV_VERSION = "VAULT_TF_KV_VERSION"
	WORKDIR             = "WORKDIR"
)

func main() {
	cfgPath := getEnvOrDefault(CONFIG_FILE, "/config.yaml")
	workdir := getEnvOrDefault(WORKDIR, "/tf-repo")
	vaultAddr := getEnvOrError(VAULT_ADDR)
	roleId := getEnvOrError(VAULT_ROLE_ID)
	secretId := getEnvOrError(VAULT_SECRET_ID)
	kvVersion := getEnvOrDefault(VAULT_TF_KV_VERSION, vaultutil.KV_V2)

	err := pkg.Run(cfgPath,
		workdir,
		vaultAddr,
		roleId,
		secretId,
		kvVersion,
	)
	if err != nil {
		log.Fatalln(err)
	}
	os.Exit(0)
}

func getEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Printf("%s not set. Using default value: `%s`", key, defaultValue)
		return defaultValue
	}
	return value
}

func getEnvOrError(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("%s is required", key)
	}
	return value
}
