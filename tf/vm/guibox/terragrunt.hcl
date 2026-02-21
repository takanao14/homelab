include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/vm"
}

inputs = {
  vms = {
    "sample" = {
      node_name        = "pve"
      config_datastore = "local-zfs"
      cores            = 4
      memory           = 8192
      qemu_guest_agent = true
      on_boot          = false
      username         = "takanao"
      ipv4             = "192.168.20.21/24"
      ipv4gw           = "192.168.20.1"
      bridge           = "vnets001"
      dns_servers      = ["192.168.10.1", "8.8.8.8"]
      scsi_hardware    = "virtio-scsi-single"
      disks = {
        scsi0 = {
          datastore_id = "local-zfs"
          size         = 100
          file_id      = "local:iso/ubuntu-24.04-xrdp.img"
          cache        = "writeback"
        }
      }
    }
  }
}
