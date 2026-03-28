locals {
  common_vars = {
    node_name        = "node1"
    config_datastore = "data-nvme"
    username         = get_env("TF_VM_USERNAME")
    ipv4gw           = "192.168.30.1"
    bridge           = "vnets30"
    scsi_hardware    = "virtio-scsi-single"
    qemu_guest_agent = true
    on_boot          = true
    os_type          = "l26"
  }
  common_disk_settings = {
    datastore_id = "data-nvme"
    cache        = "writeback"
    ssd          = true
    discard      = "on"
  }
  os_image = "local:iso/ubuntu-24.04-custom.img"
}
