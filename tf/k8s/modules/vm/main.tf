resource "proxmox_virtual_environment_vm" "vm" {
  for_each = var.vms

  name      = each.key
  node_name = each.value.node_name

  stop_on_destroy = true

  agent {
    enabled = each.value.qemu_guest_agent
  }

  on_boot = each.value.on_boot

  cpu {
    cores = each.value.cores
    type  = "host"
  }

  memory {
    dedicated = each.value.memory
  }

  bios    = "ovmf"
  machine = "q35"

  scsi_hardware = each.value.scsi_hardware

  dynamic "disk" {
    for_each = each.value.disks
    content {
      datastore_id = disk.value.datastore_id
      file_id      = disk.value.file_id
      interface    = disk.key
      size         = disk.value.size
      cache        = disk.value.cache
      file_format  = disk.value.file_format
      ssd          = disk.value.ssd
      discard      = disk.value.discard
      iothread     = true
    }
  }

  efi_disk {
    datastore_id = each.value.config_datastore
    type         = "4m"
  }

  network_device {
    bridge = each.value.bridge
    model  = "virtio"
    mtu    = 1 # Inherit MTU from bridge
  }

  initialization {
    datastore_id = each.value.config_datastore

    ip_config {
      ipv4 {
        address = each.value.ipv4
        gateway = each.value.ipv4gw
      }
    }

    dns {
      servers = each.value.dns_servers
    }

    user_account {
      username = each.value.username
      password = var.password
      keys     = [trimspace(data.local_file.ssh_public_key.content)]
    }
  }
}

