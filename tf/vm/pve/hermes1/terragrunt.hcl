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
    "hermes1" = merge(local.base_vars, {
      cores      = 2
      memory     = 4096
      on_boot    = true
      bridge     = local.common.locals.pve.net20.bridge
      ipv4       = "192.168.20.24/24"
      ipv4gw     = local.common.locals.pve.net20.ipv4gw
      cloud_init = { netplan_virtio_static = true }
      disks = {
        scsi0 = merge(local.env.locals.disk_defaults, {
          size    = 40
          file_id = "local:iso/ubuntu-26.04-base.img"
        })
      }
    })
  }
}
