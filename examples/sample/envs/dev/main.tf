terraform {
  required_version = "1.9.8"

  backend "s3" {
    bucket = "example-tfstate"
    key    = "envs/dev/terraform.tfstate"
    region = "eu-central-1"
  }
}

provider "aws" {
  region = "eu-central-1"
}

module "network" {
  source     = "../../modules/vpc"
  cidr_block = "10.1.0.0/16"
}
