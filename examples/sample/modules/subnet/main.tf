variable "cidr_block" {
  type = string
}

resource "aws_subnet" "this" {
  cidr_block = cidrsubnet(var.cidr_block, 4, 1)
}
