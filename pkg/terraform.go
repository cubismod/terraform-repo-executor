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

type TfCreds struct {
	AccessKey string
	SecretKey string
	Region    string
	Key       string
	Bucket    string
}

// standardized AppSRE terraform secret keys
const (
	AWS_ACCESS_KEY_ID     = "aws_access_key_id"
	AWS_SECRET_ACCESS_KEY = "aws_secret_access_key"
	AWS_REGION            = "region"
	AWS_BUCKET            = "bucket"
)

func extractTfCreds(secret vaultutil.VaultKvData, repo Repo) (TfCreds, error) {
	secretKeys := []string{AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY}
	errStr := "Required terraform key `%s` missing from Vault secret."
	// handle cases where a bucket & region is already defined for the AWS account via terraform-state-1.yml
	if len(repo.Bucket) > 0 && len(repo.Region) > 0 {
		for _, key := range secretKeys {
			if secret[key] == nil {
				return TfCreds{}, fmt.Errorf(errStr, key)
			}
		}
		return TfCreds{
			AccessKey: secret[AWS_ACCESS_KEY_ID].(string),
			SecretKey: secret[AWS_SECRET_ACCESS_KEY].(string),
			Bucket:    repo.Bucket,
			Region:    repo.Region,
		}, nil
	} else {
		secretKeys = append(secretKeys, []string{AWS_BUCKET, AWS_REGION}...)
		for _, key := range secretKeys {
			if secret[key] == nil {
				return TfCreds{}, fmt.Errorf(errStr, key)
			}
		}
		return TfCreds{
			AccessKey: secret[AWS_ACCESS_KEY_ID].(string),
			SecretKey: secret[AWS_SECRET_ACCESS_KEY].(string),
			Bucket:    secret[AWS_BUCKET].(string),
			Region:    secret[AWS_REGION].(string),
		}, nil
	}
}

const (
	TFVARS_FILE  = "plan.tfvars"
	BACKEND_FILE = "s3.tfbackend"
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
			BACKEND_FILE,
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

type TfVars struct {
	AccessKey     string
	SecretKey     string
	Region        string
	VaultAddress  string
	VaultRoleId   string
	VaultSecretId string
}

// generates .tfvars file to be utilized for input variables to a specific tf plan
// the generated .tfvars file will provide credentials for the aws and vault providers
// of the plan
func (e *Executor) generateTfVarsFile(creds TfCreds, repo Repo) error {
	varsTemplate := `access_key = "{{.AccessKey}}"
		{{- "\n"}}secret_key = "{{.SecretKey}}"
		{{- "\n"}}region = "{{.Region}}"
		{{- "\n"}}vault_addr = "{{.VaultAddress}}"
		{{- "\n"}}vault_role_id = "{{.VaultRoleId}}"
		{{- "\n"}}vault_secret_id = "{{.VaultSecretId}}"`

	tmpl, err := template.New("tfvars").Parse(varsTemplate)
	if err != nil {
		return err
	}

	f, err := os.Create(
		fmt.Sprintf("%s/%s/%s/%s",
			e.workdir,
			repo.Name,
			repo.Path,
			TFVARS_FILE,
		),
	)
	if err != nil {
		return err
	}
	defer f.Close()

	vars := TfVars{
		AccessKey:     creds.AccessKey,
		SecretKey:     creds.SecretKey,
		Region:        creds.Region,
		VaultAddress:  e.vaultAddr,
		VaultRoleId:   e.vaultRoleId,
		VaultSecretId: e.vaultSecretId,
	}
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

// executes target tf plan
func (e *Executor) processTfPlan(repo Repo, dryRun bool) error {
	dir := fmt.Sprintf("%s/%s/%s", e.workdir, repo.Name, repo.Path)
	tf, err := tfexec.NewTerraform(dir, "terraform")
	if err != nil {
		return err
	}

	log.Printf("Initializing terraform config for %s\n", repo.Name)
	err = tf.Init(
		context.Background(),
		tfexec.BackendConfig(BACKEND_FILE),
	)
	if err != nil {
		return err
	}

	var stdout, stderr bytes.Buffer
	tf.SetStdout(&stdout)
	tf.SetStderr(&stderr)

	planFile := fmt.Sprintf("%s/%s-plan", e.workdir, repo.Name)

	if dryRun {
		log.Println(fmt.Sprintf("Performing terraform plan for %s", repo.Name))
		_, err = tf.Plan(
			context.Background(),
			tfexec.Destroy(repo.Delete),
			tfexec.VarFile(TFVARS_FILE),
			tfexec.Out(planFile), // this plan file will be useful to have in a later improvement as well
		)
	} else {
		// tf.exec.Destroy flag cannot be passed to tf.Apply in same fashion as above Plan() logic
		if repo.Delete {
			log.Println(fmt.Sprintf("Performing terraform destroy for %s", repo.Name))
			err = tf.Destroy(
				context.Background(),
				tfexec.VarFile(TFVARS_FILE),
			)
		} else {
			log.Println(fmt.Sprintf("Performing terraform apply for %s", repo.Name))
			err = tf.Apply(
				context.Background(),
				tfexec.VarFile(TFVARS_FILE),
			)
		}

	}
	if err != nil {
		return errors.New(stderr.String())
	}

	log.Printf("Output for %s\n", repo.Name)
	log.Println(stdout.String())

	if repo.RequireFips && dryRun {
		err := e.fipsComplianceCheck(repo, planFile, tf)
		if err != nil {
			return err
		}
	}

	return nil
}
