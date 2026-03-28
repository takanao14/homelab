locals {
  vm_defaults = {
    node_name        = "pve"
    config_datastore = "local-zfs"
    qemu_guest_agent = true
    on_boot          = false
    scsi_hardware    = "virtio-scsi-single"
    bridge           = "vnets001"
    ipv4gw           = "192.168.20.1"
    username         = get_env("TF_VM_USERNAME")
  }
  disk_defaults = {
    datastore_id = "local-zfs"
    cache        = "writeback"
  }
}
