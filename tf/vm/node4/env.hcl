locals {
  vm_defaults = {
    node_name        = "node4"
    config_datastore = "data-nvme"
    username         = regex("^\\S+$", get_env("TF_VM_USERNAME"))
    scsi_hardware    = "virtio-scsi-single"
    qemu_guest_agent = true
    on_boot          = true
    os_type          = "l26"
  }
  disk_defaults = {
    datastore_id = "data-nvme"
    cache        = "writeback"
    ssd          = true
    discard      = "on"
  }
}
