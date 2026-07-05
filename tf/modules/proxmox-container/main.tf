resource "proxmox_virtual_environment_container" "container" {
  for_each = var.containers

  node_name = each.value.node_name

  unprivileged = each.value.unprivileged
  features {
    nesting = each.value.nesting
  }

  start_on_boot = each.value.on_boot

  cpu {
    cores = each.value.cores
  }

  memory {
    dedicated = each.value.memory
    swap      = each.value.swap
  }

  operating_system {
    template_file_id = each.value.os_template
    type             = each.value.os_type
  }

  dynamic "disk" {
    for_each = each.value.disks
    content {
      datastore_id = disk.value.datastore_id
      size         = disk.value.size
    }
  }

  dynamic "mount_point" {
    for_each = each.value.mount_points
    content {
      volume        = mount_point.value.volume
      path          = mount_point.value.path
      size          = mount_point.value.size
      acl           = mount_point.value.acl
      backup        = mount_point.value.backup
      mount_options = mount_point.value.mount_options
      quota         = mount_point.value.quota
      read_only     = mount_point.value.read_only
      replicate     = mount_point.value.replicate
      shared        = mount_point.value.shared
    }
  }

  network_interface {
    name   = each.value.ifname
    bridge = each.value.bridge
  }

  initialization {

    hostname = each.key

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
      password = var.password
      keys     = [trimspace(data.local_file.ssh_public_key.content)]
    }
  }
}
