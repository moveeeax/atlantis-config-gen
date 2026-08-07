terraform {
  required_version = ">= 1.5.0"

  backend "local" {
    path = "terraform.tfstate"
  }
}

provider "random" {}

resource "random_pet" "name" {
  length = 2
}
