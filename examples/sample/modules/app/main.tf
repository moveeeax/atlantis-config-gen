variable "environment" {
  type = string
}

module "app_network" {
  source     = "../vpc"
  cidr_block = "10.20.0.0/16"
}

resource "aws_instance" "app" {
  tags = {
    Name        = "app"
    Environment = var.environment
  }
}
