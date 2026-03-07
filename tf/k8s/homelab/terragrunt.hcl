include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/vm"
}

locals {
  common_vars = {
    node_name        = "node1"
    config_datastore = "data-nvme"
    username         = get_env("VM_USERNAME")
    ipv4gw           = get_env("VM_IPV4_GW")
    bridge           = get_env("VM_BRIDGE")
    dns_servers      = split(",", get_env("VM_DNS_SERVERS"))
    scsi_hardware    = "virtio-scsi-single"
    qemu_guest_agent = true
    on_boot          = true
  }

  common_disk_settings = {
    datastore_id = "data-nvme"
    cache        = "writeback"
    ssd          = true
    discard      = "on"
  }

  os_image = "local:iso/ubuntu-24.04-custom.img"
}

inputs = {
  vms = {
    "k0s-cp1" = merge(local.common_vars, {
      cores  = 2
      memory = 4096
      ipv4   = get_env("VM_CP_1_ADDR")
      disks = {
        scsi0 = merge(local.common_disk_settings, {
          size    = 40
          file_id = local.os_image
        })
      }
    })
    "k0s-worker1" = merge(local.common_vars, {
      cores  = 8
      memory = 16384
      ipv4   = get_env("VM_WORKER_1_ADDR")
      disks = {
        scsi0 = merge(local.common_disk_settings, {
          size    = 64
          file_id = local.os_image
        })
        scsi1 = merge(local.common_disk_settings, {
          size = 300
        })
      }
    })
  }
}
