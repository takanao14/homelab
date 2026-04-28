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
    "ns2" = merge(local.env.locals.container_defaults, {
      cores       = 1
      memory      = 512
      bridge      = local.common.locals.dev.net10.bridge
      ipv4        = "192.168.10.234/24"
      ipv4gw      = local.common.locals.dev.net10.ipv4gw
      dns_servers = local.common.locals.dns_external
      disks = {
        disk0 = merge(local.env.locals.disk_defaults, {
          size = 4
        })
      }
    })
    "dist2" = merge(local.env.locals.container_defaults, {
      cores       = 2
      memory      = 512
      bridge      = local.common.locals.dev.net10.bridge
      ipv4        = "192.168.10.232/24"
      ipv4gw      = local.common.locals.dev.net10.ipv4gw
      dns_servers = local.common.locals.dns_external
      disks = {
        disk0 = merge(local.env.locals.disk_defaults, {
          size = 4
        })
      }
    })
  }
}
