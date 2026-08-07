terraform {
  required_version = ">= 1.5.0"

  backend "s3" {
    bucket = "example-tfstate"
    key    = "envs/prod/terraform.tfstate"
    region = "eu-central-1"
  }
}

provider "aws" {
  region = "eu-central-1"
}

module "network" {
  source     = "../../modules/vpc"
  cidr_block = "10.0.0.0/16"
}
