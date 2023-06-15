data "vault_generic_secret" "test" {
  path = "terraform/app-sre/stage-account"
}

module "tf-executor-plan-b" {
    source= "./vpc"
    name = data.vault_generic_secret.test.data["name"]
    vpc_cidr = "10.192.0.0/16"
}
