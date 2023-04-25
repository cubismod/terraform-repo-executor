data "vault_generic_secret" "test" {
  path = "terraform/app-sre/prod-network"
}

module "tf-executor-plan-a" {
    source= "./vpc"
    name = data.vault_generic_secret.test.data["name"]
    vpc_cidr = "10.128.0.0/16"
}
