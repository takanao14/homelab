locals {
  vm_defaults = {
    node_name        = "node2"
    config_datastore = "local-lvm"
    qemu_guest_agent = true
    on_boot          = true
    scsi_hardware    = "virtio-scsi-single"
    bridge           = "vmbr0"
    ipv4gw           = "192.168.10.1"
    username         = get_env("TF_VM_USERNAME")
  }
  disk_defaults = {
    datastore_id = "local-lvm"
    cache        = "writeback"
  }
}
