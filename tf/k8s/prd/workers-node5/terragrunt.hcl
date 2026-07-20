include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/proxmox-vm"
}

locals {
  env    = read_terragrunt_config("${get_terragrunt_dir()}/env.hcl")
  common = read_terragrunt_config(find_in_parent_folders("common.hcl"))

  base_vars = merge(local.env.locals.vm_defaults, {
    dns_servers = local.common.locals.dns_internal
    dns_domain  = local.common.locals.dns_domain
  })

}

inputs = {
  vms = {
    "k0s-worker2" = merge(local.base_vars, {
      # Leave the node5 host with ~2 threads and ~3 GiB of memory.
      cores  = 10
      memory = 28672
      bridge = local.common.locals.node5.net70.bridge
      ipv4gw = local.common.locals.node5.net70.ipv4gw
      ipv4   = "192.168.70.13/24"
      disks = {
        scsi0 = merge(local.env.locals.disk_defaults, {
          size    = 64
          file_id = local.env.locals.os_image
        })
        scsi1 = merge(local.env.locals.disk_defaults, {
          size = 300
        })
      }
    })
  }
}
