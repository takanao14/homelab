locals {
  common_vars = {
    node_name        = "pve"
    config_datastore = "local-zfs"
    username         = get_env("TF_VM_USERNAME")
    ipv4gw           = "192.168.20.1"
    bridge           = "vnets001"
    scsi_hardware    = "virtio-scsi-single"
    qemu_guest_agent = true
    on_boot          = false
    os_type          = "l26"
  }
  common_disk_settings = {
    datastore_id = "local-zfs"
    cache        = "writeback"
    ssd          = true
    discard      = "on"
  }
  os_image = "local:iso/ubuntu-24.04-custom.img"
}
