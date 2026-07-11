include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/proxmox-vm"
}

locals {
  # env.hcl is read from this directory (not the parent): this stack lives in
  # the prd cluster tree but the VM is placed on pve, so it carries its own
  # host binding (see also .envrc, which sources the pve secrets).
  env    = read_terragrunt_config("${get_terragrunt_dir()}/env.hcl")
  common = read_terragrunt_config(find_in_parent_folders("common.hcl"))

  base_vars = merge(local.env.locals.vm_defaults, {
    dns_servers = local.common.locals.dns_internal
    dns_domain  = local.common.locals.dns_domain
  })
}

# prd GPU worker (ADR-0019), moved here from tf/vm/dev/gpuvm as part of the
# tf tree reorg: k0s node VMs live under tf/k8s/<cluster>. Do NOT apply this
# stack before its state has been migrated from vm/dev/gpuvm (see
# docs/plans/tf-directory-reorg.md), or it will try to create a duplicate VM.
inputs = {
  vms = {
    "gpuvm1" = merge(local.base_vars, {
      cores  = 8
      memory = 32768
      bridge = local.common.locals.pve.net20.bridge
      ipv4   = "192.168.20.22/24"
      ipv4gw = local.common.locals.pve.net20.ipv4gw
      disks = {
        scsi0 = merge(local.env.locals.disk_defaults, {
          size    = 200
          file_id = "local:iso/ubuntu-24.04-custom.img"
        })
        scsi1 = merge(local.env.locals.disk_defaults, {
          size = 300
        })
      }
      pci_devices = {
        "hostpci0" = {
          mapping = "radeon"
          pcie    = true
          rombar  = true
        }
      }
    })
  }
}
