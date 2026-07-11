locals {
  vm_defaults = {
    node_name        = "pve"
    config_datastore = "local-zfs"
    username         = get_env("TF_VM_USERNAME")
    scsi_hardware    = "virtio-scsi-single"
    qemu_guest_agent = true
    # k0s worker VM (ADR-0019): must start with the host, unlike the lab VMs
    # on this host.
    on_boot = true
    os_type = "l26"
  }
  # Kept identical to the disks as originally created on pve (no ssd/discard
  # flags): adding them here would show up as a plan diff on the live VM.
  disk_defaults = {
    datastore_id = "local-zfs"
    cache        = "writeback"
  }
}
