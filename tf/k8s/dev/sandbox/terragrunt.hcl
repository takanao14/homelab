include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/proxmox-vm"
}

locals {
  env = read_terragrunt_config(find_in_parent_folders("env.hcl"))
  common = read_terragrunt_config(find_in_parent_folders("common.hcl"))

  base_vars = merge(local.env.locals.vm_defaults, {
    dns_servers = local.common.locals.dns_internal
    dns_domain  = local.common.locals.dns_domain
  })
}

inputs = {
  vms = {
    "sbox-k0s-cp1" = merge(local.base_vars, {
      cores  = 2
      memory = 4096
      bridge = local.common.locals.dev.net20.bridge
      ipv4gw = local.common.locals.dev.net20.ipv4gw
      ipv4   = "192.168.20.31/24"
      disks = {
        scsi0 = merge(local.env.locals.disk_defaults, {
          size    = 40
          file_id = local.env.locals.os_image
        })
      }
    })
    "sbox-k0s-worker1" = merge(local.base_vars, {
      cores  = 2
      memory = 4096
      bridge = local.common.locals.dev.net20.bridge
      ipv4gw = local.common.locals.dev.net20.ipv4gw
      ipv4   = "192.168.20.32/24"
      disks = {
        scsi0 = merge(local.env.locals.disk_defaults, {
          size    = 40
          file_id = local.env.locals.os_image
        })
        scsi1 = merge(local.env.locals.disk_defaults, {
          size = 40
        })
      }
    })
    "sbox-k0s-worker2" = merge(local.base_vars, {
      cores  = 2
      memory = 4096
      bridge = local.common.locals.dev.net20.bridge
      ipv4gw = local.common.locals.dev.net20.ipv4gw
      ipv4   = "192.168.20.33/24"
      disks = {
        scsi0 = merge(local.env.locals.disk_defaults, {
          size    = 40
          file_id = local.env.locals.os_image
        })
        scsi1 = merge(local.env.locals.disk_defaults, {
          size = 40
        })
      }
    })
    "sbox-k0s-worker3" = merge(local.base_vars, {
      cores  = 2
      memory = 4096
      bridge = local.common.locals.dev.net20.bridge
      ipv4gw = local.common.locals.dev.net20.ipv4gw
      ipv4   = "192.168.20.34/24"
      disks = {
        scsi0 = merge(local.env.locals.disk_defaults, {
          size    = 40
          file_id = local.env.locals.os_image
        })
        scsi1 = merge(local.env.locals.disk_defaults, {
          size = 40
        })
      }
    })
  }
}
