// main package for terraform-repo-executor
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

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
	TfParallelism  = "TF_PARALLELISM"
	LogFlushDelay  = "LOG_FLUSH_DELAY_SECONDS"
)

func main() {
	// Generate unique session ID for Vector log tracking
	sessionID := fmt.Sprintf("session-%d", time.Now().UnixNano())

	log.Printf("Starting terraform-repo-executor [%s]", sessionID)

	// Setup graceful shutdown handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	done := make(chan error, 1)

	cfgPath := getEnvOrDefault(ConfigFile, "/config.yaml")
	workdir := getEnvOrDefault(WorkDir, "/tmp/tf-repo")
	vaultAddr := getEnvOrError(VaultAddr)
	roleID := getEnvOrError(VaultRoleID)
	secretID := getEnvOrError(VaultSecretID)
	gitlabLogRepo := getEnvOrError(GitlabLogRepo)
	gitlabUsername := getEnvOrError(GitlabUsername)
	gitlabToken := getEnvOrError(GitlabToken)
	gitEmail := getEnvOrError(GitEmail)
	tfParallelism := getEnvOrDefault(TfParallelism, "10")
	logFlushDelay := getEnvOrDefault(LogFlushDelay, "3")

	tfParallelismInt, err := strconv.Atoi(tfParallelism)
	if err != nil {
		log.Fatal("Integer value required for `TF_PARALLELISM` environment variable")
	}

	logFlushDelayInt, err := strconv.Atoi(logFlushDelay)
	if err != nil {
		logFlushDelayInt = 3
	}

	// Run main logic in goroutine
	go func() {
		err := pkg.Run(cfgPath,
			workdir,
			vaultAddr,
			roleID,
			secretID,
			gitlabLogRepo,
			gitlabUsername,
			gitlabToken,
			gitEmail,
			tfParallelismInt,
		)
		done <- err
	}()

	// Wait for completion or signal
	select {
	case err := <-done:
		if err != nil {
			log.Printf("Error: %v [%s]", err, sessionID)
			performGracefulShutdown(sessionID, logFlushDelayInt)
			os.Exit(1)
		}
		log.Printf("Completed successfully [%s]", sessionID)
	case sig := <-sigChan:
		log.Printf("Received signal %v, shutting down [%s]", sig, sessionID)
	}

	performGracefulShutdown(sessionID, logFlushDelayInt)
	os.Exit(0)
}

func performGracefulShutdown(sessionID string, delaySeconds int) {
	// Add minimal logging noise to help Vector fingerprinting
	log.Printf("Shutting down [%s] - %s", sessionID, time.Now().Format(time.RFC3339))

	// Brief delay for log collection
	if delaySeconds > 0 {
		time.Sleep(time.Duration(delaySeconds) * time.Second)
	}

	log.Printf("Shutdown complete [%s]", sessionID)
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
