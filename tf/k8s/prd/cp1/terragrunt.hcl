include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/proxmox-vm"
}

locals {
  # env.hcl is read from this directory (not the parent): this stack lives in
  # the prd cluster tree but the VM is placed on node4, so it carries its own
  # host binding (see also .envrc, which sources the node4 secrets).
  env    = read_terragrunt_config("${get_terragrunt_dir()}/env.hcl")
  common = read_terragrunt_config(find_in_parent_folders("common.hcl"))

  base_vars = merge(local.env.locals.vm_defaults, {
    dns_servers = local.common.locals.dns_internal
    dns_domain  = local.common.locals.dns_domain
  })
}

# prd k0s controller relocated from node1 (../prd-cluster, 192.168.30.11).
# Same spec as the original cp1. The old VM definition is removed from
# ../prd-cluster only after the k0s backup/restore cutover succeeds
# (see docs/plans/control-plane-relocation.md).
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
