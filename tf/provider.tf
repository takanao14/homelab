terraform {
  required_providers {
    proxmox = {
      source  = "bpg/proxmox"
      version = "~> 0.114"
    }
    local = {
      source  = "hashicorp/local"
      version = "~> 2.9"
    }
  }
}

provider "proxmox" {
}

variable "proxmox_ssh_username" {
  type    = string
  default = "root"
}

provider "proxmox" {
  alias = "snippets"

  ssh {
    agent    = true
    username = var.proxmox_ssh_username
  }
}
