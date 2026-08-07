variable "cidr_block" {
  type = string
}

# vpc composes the subnet module: transitive dependency for any root using vpc.
module "subnet" {
  source     = "../subnet"
  cidr_block = var.cidr_block
}

resource "aws_vpc" "this" {
  cidr_block = var.cidr_block
}
