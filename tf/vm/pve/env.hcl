locals {
  vm_defaults = {
    node_name        = "pve"
    config_datastore = "local-zfs"
    username         = get_env("TF_VM_USERNAME")
    scsi_hardware    = "virtio-scsi-single"
    qemu_guest_agent = true
    on_boot          = false
    os_type          = "l26"
  }
  disk_defaults = {
    datastore_id = "local-zfs"
    cache        = "writeback"
  }
}
