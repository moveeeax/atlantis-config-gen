terraform {
  required_version = ">= 1.5.0"

  backend "s3" {
    bucket = "example-tfstate"
    key    = "services/api/terraform.tfstate"
    region = "eu-central-1"
  }
}

provider "aws" {
  region = var.region
}

# Pulls in modules/app, which itself uses modules/vpc -> modules/subnet.
# A change in any of those must replan this project.
module "app" {
  source      = "../../modules/app"
  environment = var.environment
}
