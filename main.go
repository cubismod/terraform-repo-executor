// main package for terraform-repo-executor
package main

import (
	"log"
	"os"

	"github.com/app-sre/terraform-repo-executor/pkg"
)

// environment variables
const (
	ConfigFile     = "CONFIG_FILE"
	VaultAddr      = "VAULT_ADDR"
	VaultRoleID    = "VAULT_ROLE_ID"
	VaultSecretID  = "VAULT_SECRET_ID"
	WorkDir        = "WORKDIR"
	GitlabLogRepo  = "GITLAB_LOG_REPO"
	GitlabUsername = "GITLAB_USERNAME"
	GitlabToken    = "GITLAB_TOKEN"
	GitEmail       = "GIT_EMAIL"
)

func main() {
	cfgPath := getEnvOrDefault(ConfigFile, "/config.yaml")
	workdir := getEnvOrDefault(WorkDir, "/tf-repo")
	vaultAddr := getEnvOrError(VaultAddr)
	roleID := getEnvOrError(VaultRoleID)
	secretID := getEnvOrError(VaultSecretID)
	gitlabLogRepo := getEnvOrError(GitlabLogRepo)
	gitlabUsername := getEnvOrError(GitlabUsername)
	gitlabToken := getEnvOrError(GitlabToken)
	gitEmail := getEnvOrError(GitEmail)

	err := pkg.Run(cfgPath,
		workdir,
		vaultAddr,
		roleID,
		secretID,
		gitlabLogRepo,
		gitlabUsername,
		gitlabToken,
		gitEmail,
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
