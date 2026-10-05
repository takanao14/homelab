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
    "openclaw1" = merge(local.base_vars, {
      cores   = 2
      memory  = 4096
      on_boot = true
      bridge  = local.common.locals.pve.net20.bridge
      cloud_init = {
        network_data = yamlencode({
          version  = 2
          renderer = "networkd"
          ethernets = {
            primary = {
              # The VM has one VirtIO NIC; its PCI-derived name can change.
              match     = { driver = "virtio_net" }
              addresses = ["192.168.20.23/24"]
              routes = [{
                to  = "default"
                via = local.common.locals.pve.net20.ipv4gw
              }]
              nameservers = {
                addresses = local.common.locals.dns_internal
                search    = [local.common.locals.dns_domain]
              }
            }
          }
        })
      }
      disks = {
        scsi0 = merge(local.env.locals.disk_defaults, {
          size    = 40
          file_id = "local:iso/ubuntu-26.04-base.img"
        })
      }
    })
  }
}
