include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/proxmox-container"
}

locals {
  env    = read_terragrunt_config(find_in_parent_folders("env.hcl"))
  common = read_terragrunt_config(find_in_parent_folders("common.hcl"))
}

inputs = {
  containers = {
    "devdns" = merge(local.env.locals.container_defaults, {
      ipv4        = "192.168.10.243/24"
      dns_servers = local.common.locals.dns_external
      disks = {
        disk0 = local.env.locals.disk_defaults
      }
    })
  }
}
