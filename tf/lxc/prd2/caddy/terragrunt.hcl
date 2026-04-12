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
    "caddy" = merge(local.env.locals.container_defaults, {
      cores       = 1
      memory      = 512
      bridge      = local.common.locals.prd2.net10.bridge
      ipv4        = "192.168.10.244/24"
      ipv4gw      = local.common.locals.prd2.net10.ipv4gw
      dns_servers = local.common.locals.dns_external
      disks = {
        disk0 = merge(local.env.locals.disk_defaults, {
          size = 8
        })
      }
    })
  }
}
