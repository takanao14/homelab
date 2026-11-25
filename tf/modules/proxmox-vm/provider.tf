# Satisfies standalone module validation (`tofu test`); Terragrunt overwrites
# this file with `tf/provider.tf` in `.terragrunt-cache`.
terraform {
  required_providers {
    proxmox = {
      source = "bpg/proxmox"
    }
    local = {
      source = "hashicorp/local"
    }
  }
}

provider "proxmox" {
  alias = "snippets"
}
