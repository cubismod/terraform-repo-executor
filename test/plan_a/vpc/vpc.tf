variable vpc_cidr {}
variable name {}

resource "aws_vpc" "main" {
  cidr_block       = var.vpc_cidr

  tags = {
    Name = var.name
  }
}