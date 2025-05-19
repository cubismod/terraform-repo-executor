package pkg

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"text/template"
	"time"

	"github.com/app-sre/terraform-repo-executor/pkg/vaultutil"
	"github.com/hashicorp/terraform-exec/tfexec"
)

// TfCreds is made up of AWS credentials and configuration for using an S3 backend with Terraform
type TfCreds struct {
	AccessKey string
	SecretKey string
	Region    string
	Key       string // set when initializing backend
	Bucket    string
}

// standardized AppSRE terraform secret keys
const (
	AwsAccessKeyID     = "aws_access_key_id"
	AwsSecretAccessKey = "aws_secret_access_key"
	AwsRegion          = "region"
	AwsBucket          = "bucket"
)

func extractTfCreds(secret vaultutil.VaultKvData, repo Repo) (TfCreds, error) {
	secretKeys := []string{AwsAccessKeyID, AwsSecretAccessKey}
	errStr := "Required terraform key `%s` missing from Vault secret."
	// handle cases where a bucket & region is already defined for the AWS account via terraform-state-1.yml
	if len(repo.Bucket) > 0 && len(repo.Region) > 0 {
		for _, key := range secretKeys {
			if secret[key] == nil {
				return TfCreds{}, fmt.Errorf(errStr, key)
			}
		}
		return TfCreds{
			AccessKey: secret[AwsAccessKeyID].(string),
			SecretKey: secret[AwsSecretAccessKey].(string),
			Bucket:    repo.Bucket,
			Region:    repo.Region,
		}, nil
	}
	secretKeys = append(secretKeys, []string{AwsBucket, AwsRegion}...)
	for _, key := range secretKeys {
		if secret[key] == nil {
			return TfCreds{}, fmt.Errorf(errStr, key)
		}
	}
	return TfCreds{
		AccessKey: secret[AwsAccessKeyID].(string),
		SecretKey: secret[AwsSecretAccessKey].(string),
		Bucket:    secret[AwsBucket].(string),
		Region:    secret[AwsRegion].(string),
	}, nil
}

// terraform specific filenames
// the "auto" vars files will automatically be loaded by the tf binary
const (
	AWSVarsFile   = "aws.auto.tfvars"
	InputVarsFile = "input.auto.tfvars"
	BackendFile   = "s3.tfbackend"
)

// generates a .tfbackend file to be utilized as partial backend config input file
// the generated backend file will provide credentials for an s3 backend config
func (e *Executor) generateBackendFile(creds TfCreds, repo Repo) error {
	backendTemplate := `access_key = "{{.AccessKey}}"
		{{- "\n"}}secret_key = "{{.SecretKey}}"
		{{- "\n"}}region = "{{.Region}}"
		{{- "\n"}}key = "{{.Key}}"
		{{- "\n"}}bucket = "{{.Bucket}}"`

	err := WriteTemplate(creds, backendTemplate, fmt.Sprintf("%s/%s/%s/%s", e.workdir, repo.Name, repo.Path, BackendFile))

	if err != nil {
		return err
	}

	return nil
}

// TfVars are secrets and IDs required for setting up a Terraform S3 backend
type TfVars struct {
	AccessKey     string
	SecretKey     string
	Region        string
	VaultAddress  string
	VaultRoleID   string
	VaultSecretID string
}

// generates a .tfvars file including Vault & S3 backend credentials
// TODO: add test case around this function
func (e *Executor) generateCredVarsFile(creds TfCreds, repo Repo) error {
	// first create a *.tfvars file for S3 backend credentials
	body := `access_key = "{{.AccessKey}}"
		{{- "\n"}}secret_key = "{{.SecretKey}}"
		{{- "\n"}}region = "{{.Region}}"
		{{- "\n"}}vault_addr = "{{.VaultAddress}}"
		{{- "\n"}}vault_role_id = "{{.VaultRoleID}}"
		{{- "\n"}}vault_secret_id = "{{.VaultSecretID}}"`

	tfVars := TfVars{
		AccessKey:     creds.AccessKey,
		SecretKey:     creds.SecretKey,
		Region:        creds.Region,
		VaultAddress:  e.vaultAddr,
		VaultRoleID:   e.vaultRoleID,
		VaultSecretID: e.vaultSecretID,
	}

	err := WriteTemplate(tfVars, body, fmt.Sprintf("%s/%s/%s/%s", e.workdir, repo.Name, repo.Path, AWSVarsFile))

	if err != nil {
		return err
	}

	return nil
}

// generates a .tfvars file including input variables from Vault
func (e *Executor) generateInputVarsFile(data vaultutil.VaultKvData, repo Repo) error {
	body := `{{ range $k, $v := . }}{{ $k }} = "{{ $v }}"{{- "\n"}}{{ end }}`

	err := WriteTemplate(data, body, fmt.Sprintf("%s/%s/%s/%s", e.workdir, repo.Name, repo.Path, InputVarsFile))

	if err != nil {
		return err
	}

	return nil
}

// WriteTemplate is responsible for templating a file and writing it to the location specified at out
// note that this is not a struct method as generics are incompatible with methods
func WriteTemplate[T TfVars | vaultutil.VaultKvData | TfCreds | StateVars](inputs T, body string, out string) error {
	tmpl, err := template.New(out).Parse(body)
	if err != nil {
		return err
	}

	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()

	err = tmpl.Execute(f, inputs)
	if err != nil {
		return err
	}

	return nil
}

// checks the generated terraform plan file to ensure that the fips endpoint is enabled in the AWS provider configuration
func (e *Executor) fipsComplianceCheck(repo Repo, planFile string, tf *tfexec.Terraform) error {
	out, err := tf.ShowPlanFile(context.Background(), planFile)

	if err != nil {
		log.Println("Unable to determine FIPS compatibility")
		return err
	}

	compliant := false

	for _, provider := range out.Config.ProviderConfigs {
		if provider.Name == "aws" {
			for k, v := range provider.Expressions {
				if k == "use_fips_endpoint" && v.ConstantValue == true {
					compliant = true
				}
			}
		}
	}

	if !compliant {
		return fmt.Errorf("repository '%s' is not using 'use_fips_endpoint = true' for the AWS provider despite the repo requiring fips", repo.Name)
	}

	return nil
}

// performs a terraform show without the `-json` flag to workaround the fact that the tfexec package
// only supports outputting the state as JSON which exposes sensitive values
func (e *Executor) showRaw(dir string, tfBinaryLocation string) (string, error) {
	out, err := executeCommand(dir, tfBinaryLocation, []string{"show"})
	if err != nil {
		return "", err
	}
	return out, err
}

// our jenkins instances are set to time out after 30 minutes of no logs to the console so this simply
// prints out a log message once a minute that the apply/plan/destroy is still in progress so pipelines don't time out
func reportProgress(done <-chan bool, repoName string) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			log.Printf("Terraform action completed for %s", repoName)
			return
		case <-ticker.C:
			log.Printf("Terraform action is still running for %s...", repoName)
		}
	}
}

// performs a terraform plan and then apply if not running in dry run mode
// additionally captures any tf outputs if necessary
func (e *Executor) processTfPlan(repo Repo, dryRun bool, envVars map[string]string) (map[string]tfexec.OutputMeta, error) {
	dir := fmt.Sprintf("%s/%s/%s", e.workdir, repo.Name, repo.Path)

	// each repo can use a different version of the TF binary, specified in App Interface
	tfBinaryLocation := fmt.Sprintf("/usr/bin/Terraform/%s/terraform", repo.TfVersion)

	tf, err := tfexec.NewTerraform(dir, tfBinaryLocation)
	if err != nil {
		return nil, err
	}

	log.Printf("Initializing terraform config for %s\n", repo.Name)
	err = tf.Init(
		context.Background(),
		tfexec.BackendConfig(BackendFile),
	)
	if err != nil {
		return nil, err
	}
	tf.SetStdout(os.Stdout)
	tf.SetStderr(os.Stderr)

	var blackhole bytes.Buffer
	// supply aws access key, secret key variables to the terraform executable for remote_backend_state
	err = tf.SetEnv(envVars)
	if err != nil {
		return nil, err
	}

	planFile := fmt.Sprintf("%s/%s-plan", e.workdir, repo.Name)
	var output map[string]tfexec.OutputMeta

	done := make(chan bool)

	if dryRun {
		log.Printf("Performing terraform plan for %s", repo.Name)
		go reportProgress(done, repo.Name)
		_, err = tf.Plan(
			context.Background(),
			tfexec.Destroy(repo.Delete),
			tfexec.Out(planFile), // this plan file will be useful to have in a later improvement as well
		)
	} else {
		// tf.exec.Destroy flag cannot be passed to tf.Apply in same fashion as above Plan() logic
		if repo.Delete {
			log.Printf("Performing terraform destroy for %s", repo.Name)
			go reportProgress(done, repo.Name)
			err = tf.Destroy(
				context.Background(),
			)
		} else {
			log.Printf("Performing terraform apply for %s", repo.Name)
			go reportProgress(done, repo.Name)
			err = tf.Apply(
				context.Background(),
			)

			if repo.TfVariables.Outputs.Path != "" {
				log.Printf("Capturing Output values to save to %s in Vault", repo.TfVariables.Outputs.Path)
				// don't log the results of `terraform output -json` as that can leak sensitive credentials
				tf.SetStdout(&blackhole)
				tf.SetStderr(&blackhole)
				output, err = tf.Output(
					context.Background(),
				)
			}
		}

	}
	close(done)
	if err != nil {
		return nil, err
	}

	if !dryRun {
		rawState, err := e.showRaw(dir, tfBinaryLocation)
		if err != nil {
			return nil, err
		}
		err = e.commitAndPushState(repo, rawState)
		if err != nil {
			log.Printf("Unable to commit state file to Git, error: %s", err)
		}
	}

	if repo.RequireFips && dryRun {
		err = e.fipsComplianceCheck(repo, planFile, tf)
		if err != nil {
			return nil, err
		}
	}

	return output, nil
}
