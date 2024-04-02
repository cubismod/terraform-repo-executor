package pkg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"text/template"

	"github.com/app-sre/terraform-repo-executor/pkg/vaultutil"
	"github.com/hashicorp/terraform-exec/tfexec"
)

// TfCreds is made up of AWS credentials and configuration for using an S3 backend with Terraform
type TfCreds struct {
	AccessKey string
	SecretKey string
	Region    string
	Key       string
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

	tmpl, err := template.New("backend").Parse(backendTemplate)
	if err != nil {
		return err
	}

	f, err := os.Create(
		fmt.Sprintf("%s/%s/%s/%s",
			e.workdir,
			repo.Name,
			repo.Path,
			BackendFile,
		),
	)
	if err != nil {
		return err
	}
	defer f.Close()

	err = tmpl.Execute(f, creds)
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

// generates .tfvars file to be utilized for input variables to a specific tf plan
// the generated .tfvars file will provide credentials for the aws and vault providers
// of the plan
func (e *Executor) generateTfVarsFile(creds TfCreds, repo Repo) error {
	// first create a *.tfvars file for S3 backend credentials
	body := `access_key = "{{.AccessKey}}"
		{{- "\n"}}secret_key = "{{.SecretKey}}"
		{{- "\n"}}region = "{{.Region}}"
		{{- "\n"}}vault_addr = "{{.VaultAddress}}"
		{{- "\n"}}vault_role_id = "{{.VaultRoleId}}"
		{{- "\n"}}vault_secret_id = "{{.VaultSecretId}}"`

	filename := fmt.Sprintf("%s/%s/%s/%s",
		e.workdir,
		repo.Name,
		repo.Path,
		AWSVarsFile,
	)

	vars := TfVars{
		AccessKey:     creds.AccessKey,
		SecretKey:     creds.SecretKey,
		Region:        creds.Region,
		VaultAddress:  e.vaultAddr,
		VaultRoleID:   e.vaultRoleID,
		VaultSecretID: e.vaultSecretID,
	}

	err := generateVarsTemplate(vars, "aws", body, repo, filename)

	if err != nil {
		return err
	}

	return nil
}

func (e *Executor) generateInputVarsFile(data vaultutil.VaultKvData, repo Repo) error {
	body := `{{ range $k, $v := . }}
		{{ $k }} = "{{ $v }}"
		{{ end }}`

	filename := fmt.Sprintf("%s/%s/%s/%s",
		e.workdir,
		repo.Name,
		repo.Path,
		InputVarsFile,
	)

	err := generateVarsTemplate(data, "input", body, repo, filename)

	if err != nil {
		return err
	}

	return nil
}

// generates a .tfvars file at the specified filename which is used for AWS credentials or loading Terraform input variables
func generateVarsTemplate[T TfVars | vaultutil.VaultKvData](vars T, name string, body string, repo Repo, filename string) error {
	tmpl, err := template.New(name).Parse(body)
	if err != nil {
		return err
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	err = tmpl.Execute(f, vars)
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

// performs a terraform plan and then apply if not running in dry run mode
// additionally captures any tf outputs if necessary
func (e *Executor) processTfPlan(repo Repo, dryRun bool) (map[string]tfexec.OutputMeta, error) {
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

	var stdout, stderr bytes.Buffer
	tf.SetStdout(&stdout)
	tf.SetStderr(&stderr)

	planFile := fmt.Sprintf("%s/%s-plan", e.workdir, repo.Name)

	if dryRun {
		log.Printf("Performing terraform plan for %s", repo.Name)
		_, err = tf.Plan(
			context.Background(),
			tfexec.Destroy(repo.Delete),
			tfexec.Out(planFile), // this plan file will be useful to have in a later improvement as well
		)
	} else {
		// tf.exec.Destroy flag cannot be passed to tf.Apply in same fashion as above Plan() logic
		if repo.Delete {
			log.Printf("Performing terraform destroy for %s", repo.Name)
			err = tf.Destroy(
				context.Background(),
			)
		} else {
			log.Printf("Performing terraform apply for %s", repo.Name)
			err = tf.Apply(
				context.Background(),
			)

			if repo.TfVariables.Outputs.Path != "" {
				log.Printf("Capturing Output values to save to %s in Vault", repo.TfVariables.Outputs.Path)
				output, err := tf.Output(
					context.Background(),
				)
				if err != nil {
					return nil, err
				}
				return output, nil
			}
		}

	}
	if err != nil {
		return nil, errors.New(stderr.String())
	}

	log.Printf("Output for %s\n", repo.Name)
	log.Println(stdout.String())

	if repo.RequireFips && dryRun {
		err := e.fipsComplianceCheck(repo, planFile, tf)
		if err != nil {
			return nil, err
		}
	}

	return nil, nil
}
