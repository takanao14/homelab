locals {
  vm_defaults = {
    node_name        = "pve"
    config_datastore = "local-zfs"
    qemu_guest_agent = true
    on_boot          = false
    scsi_hardware    = "virtio-scsi-single"
    username         = get_env("TF_VM_USERNAME")
  }
  disk_defaults = {
    datastore_id = "local-zfs"
    cache        = "writeback"
  }
}
