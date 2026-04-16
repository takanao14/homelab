locals {
  vm_defaults = {
    node_name        = "node2"
    config_datastore = "local-lvm"
    username         = get_env("TF_VM_USERNAME")
    scsi_hardware    = "virtio-scsi-single"
    qemu_guest_agent = true
    on_boot          = true
    os_type          = "l26"
  }
  disk_defaults = {
    datastore_id = "local-lvm"
    cache        = "writeback"
    ssd          = true
    discard      = "on"
  }
}
