include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/proxmox-vm"
}

locals {
  env = read_terragrunt_config("${get_terragrunt_dir()}/env.hcl")
  common = read_terragrunt_config(find_in_parent_folders("common.hcl"))

  base_vars = merge(local.env.locals.common_vars, {
    dns_servers = local.common.locals.dns_internal
    dns_domain  = local.common.locals.dns_domain
  })
}

inputs = {
  vms = {
    "k0s-cp1" = merge(local.base_vars, {
      cores  = 2
      memory = 4096
      ipv4   = "192.168.30.11/24"
      disks = {
        scsi0 = merge(local.env.locals.common_disk_settings, {
          size    = 40
          file_id = local.env.locals.os_image
        })
      }
    })
    "k0s-worker1" = merge(local.base_vars, {
      cores  = 8
      memory = 16384
      ipv4   = "192.168.30.12/24"
      disks = {
        scsi0 = merge(local.env.locals.common_disk_settings, {
          size    = 64
          file_id = local.env.locals.os_image
        })
        scsi1 = merge(local.env.locals.common_disk_settings, {
          size = 300
        })
      }
    })
  }
}
