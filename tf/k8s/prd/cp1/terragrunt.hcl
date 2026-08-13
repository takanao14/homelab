include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/proxmox-vm"
}

locals {
  # This stack overrides the cluster default with node4 host binding and secrets.
  env    = read_terragrunt_config("${get_terragrunt_dir()}/env.hcl")
  common = read_terragrunt_config(find_in_parent_folders("common.hcl"))

  base_vars = merge(local.env.locals.vm_defaults, {
    dns_servers = local.common.locals.dns_internal
    dns_domain  = local.common.locals.dns_domain
  })
}

inputs = {
  vms = {
    "k0s-cp1" = merge(local.base_vars, {
      cores  = 2
      memory = 4096
      bridge = local.common.locals.node4.net60.bridge
      ipv4gw = local.common.locals.node4.net60.ipv4gw
      ipv4   = "192.168.60.11/24"
      disks = {
        scsi0 = merge(local.env.locals.disk_defaults, {
          size    = 40
          file_id = local.env.locals.os_image
        })
      }
    })
  }
}
