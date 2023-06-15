terraform {
    backend "s3" {}
    required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.0"
    }
  }
}

variable access_key {}
variable secret_key {}
variable region {}
provider "aws" {
  access_key = var.access_key
  secret_key = var.secret_key
  region     = var.region
}

variable vault_addr {}
variable vault_role_id {}
variable vault_secret_id {}
provider "vault" {
  address=var.vault_addr
  # https://stackoverflow.com/questions/73034161/permission-denied-on-vault-terraform-provider-token-creation
  skip_child_token = true 
  auth_login {
    path = "auth/approle/login"

    parameters = {
      role_id   = var.vault_role_id
      secret_id = var.vault_secret_id
    }
  }
}

