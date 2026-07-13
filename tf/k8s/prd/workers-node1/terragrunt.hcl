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
    "k0s-worker1" = merge(local.base_vars, {
      # node1 hosts only this worker since the CP moved to node4 (see cp1),
      # so its freed 2 vCPU / 4GB are reclaimed here. Sized to leave the PVE
      # host ~2 threads and ~3GB on a 12-thread / 32GB node1.
      cores  = 10
      memory = 28672
      bridge = local.common.locals.node1.net30.bridge
      ipv4gw = local.common.locals.node1.net30.ipv4gw
      ipv4   = "192.168.30.12/24"
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
