include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/proxmox-vm"
}

locals {
  env     = read_terragrunt_config(find_in_parent_folders("env.hcl"))
  common  = read_terragrunt_config(find_in_parent_folders("common.hcl"))
  freebsd = read_terragrunt_config("${get_terragrunt_dir()}/cloudinit.hcl")
}

inputs = {
  vms = {
    "freebsd" = merge(local.env.locals.vm_defaults, {
      cores            = 2
      memory           = 4096
      on_boot          = true
      started          = true
      qemu_guest_agent = false
      os_type          = "other"
      bridge           = local.common.locals.pve.net10.bridge
      config_interface = "ide2"
      disks = {
        scsi0 = merge(local.env.locals.disk_defaults, {
          size    = 40
          file_id = "local:iso/freebsd-15.1-cloudinit-ufs.img"
        })
      }
      cloud_init = {
        type = "nocloud"
        user_data = "#cloud-config\n${yamlencode(merge(local.freebsd.locals.user_config, {
          hostname = "freebsd"
          fqdn     = "freebsd.${local.common.locals.dns_domain}"
        }))}"
        network_data = yamlencode({
          version = 2
          ethernets = {
            vtnet0 = merge(local.freebsd.locals.ethernet_config, {
              addresses = ["192.168.10.183/24"]
            })
          }
        })
      }
    })
  }
}
