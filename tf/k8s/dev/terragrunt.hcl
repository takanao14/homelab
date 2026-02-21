include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/vm"
}

locals {
  common_vars = {
    node_name        = "pve"
    config_datastore = "local-zfs"
    username         = "takanao"
    ipv4gw           = "192.168.20.1"
    bridge           = "vnets001"
    dns_servers      = ["192.168.10.1", "8.8.8.8"]
    scsi_hardware    = "virtio-scsi-single"
    qemu_guest_agent = true
    on_boot          = false
  }

  common_disk_settings = {
    datastore_id = "local-zfs"
    cache        = "writeback"
    ssd          = true
    discard      = "on"
  }

  os_image = "local:iso/ubuntu-24.04-custom.img"
}

inputs = {
  vms = {
    "dev-k0s-cp1" = merge(local.common_vars, {
      cores  = 2
      memory = 4096
      ipv4   = "192.168.20.11/24"
      disks = {
        scsi0 = merge(local.common_disk_settings, {
          size    = 40
          file_id = local.os_image
        })
      }
    })
    "dev-k0s-worker1" = merge(local.common_vars, {
      cores  = 8
      memory = 8192
      ipv4   = "192.168.20.12/24"
      disks = {
        scsi0 = merge(local.common_disk_settings, {
          size    = 64
          file_id = local.os_image
        })
        scsi1 = merge(local.common_disk_settings, {
          size = 100
        })
      }
    })
  }
}
