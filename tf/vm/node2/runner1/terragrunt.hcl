include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/proxmox-vm"
}

locals {
  env    = read_terragrunt_config(find_in_parent_folders("env.hcl"))
  common = read_terragrunt_config(find_in_parent_folders("common.hcl"))

  base_vars = merge(local.env.locals.vm_defaults, {
    dns_servers = local.common.locals.dns_internal
    dns_domain  = local.common.locals.dns_domain
  })
}

inputs = {
  vms = {
    "runner1" = merge(local.base_vars, {
      cores   = 2
      memory  = 4096
      bridge  = local.common.locals.node2.net10.bridge
      ipv4    = "192.168.10.246/24"
      ipv4gw  = local.common.locals.node2.net10.ipv4gw
      disks = {
        scsi0 = merge(local.env.locals.disk_defaults, {
          size    = 40
          file_id = "local:iso/ubuntu-24.04-custom.img"
        })
      }
    })
  }
}
