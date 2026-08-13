locals {
  vm_defaults = {
    node_name        = "pve"
    config_datastore = "local-zfs"
    username         = get_env("TF_VM_USERNAME")
    scsi_hardware    = "virtio-scsi-single"
    qemu_guest_agent = true
    # k0s workers start with the host (ADR-0019).
    on_boot = true
    os_type = "l26"
  }
  # Preserve the live pve disk settings; ssd/discard would create drift.
  disk_defaults = {
    datastore_id = "local-zfs"
    cache        = "writeback"
  }
}
