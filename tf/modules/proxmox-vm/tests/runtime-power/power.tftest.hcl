mock_provider "proxmox" {}
mock_provider "proxmox" {
  alias = "snippets"
}
mock_provider "local" {
  mock_data "local_file" {
    defaults = {
      content = "ssh-ed25519 test-only"
    }
  }
}

variables {
  password       = "test-only"
  ssh_public_key = "/unused"
  vms = {
    test = {
      node_name        = "test"
      config_datastore = "local-zfs"
      cores            = 4
      memory           = 8192
      qemu_guest_agent = false
      on_boot          = false
      started          = false
      username         = "original"
      ipv4             = "192.0.2.2/24"
      ipv4gw           = "192.0.2.1"
      bridge           = "vmbr0"
      os_type          = "l26"
      disks = {
        scsi0 = {
          datastore_id = "local-zfs"
          size         = 32
          file_id      = "local:iso/original.img"
        }
      }
    }
  }
}

run "create_stopped" {
  command = apply

  assert {
    condition     = !proxmox_virtual_environment_vm.vm["test"].started && !proxmox_virtual_environment_vm.vm["test"].on_boot
    error_message = "Externally powered VMs must be created stopped without host autostart."
  }
}

run "ignore_runtime_power_and_preserve_module_ignores" {
  command = plan

  variables {
    vms = {
      test = {
        node_name        = "test"
        config_datastore = "local-zfs"
        cores            = 4
        memory           = 8192
        qemu_guest_agent = false
        on_boot          = false
        started          = true
        username         = "changed"
        ipv4             = "192.0.2.3/24"
        ipv4gw           = "192.0.2.1"
        bridge           = "vmbr0"
        os_type          = "l26"
        disks = {
          scsi0 = {
            datastore_id = "local-zfs"
            size         = 32
            file_id      = "local:iso/changed.img"
          }
        }
      }
    }
  }

  assert {
    condition     = !proxmox_virtual_environment_vm.vm["test"].started
    error_message = "The lifecycle override must leave runtime power state unchanged."
  }

  assert {
    condition     = proxmox_virtual_environment_vm.vm["test"].disk[0].file_id == "local:iso/original.img"
    error_message = "The lifecycle override must preserve the module's disk image ignore."
  }

  assert {
    condition     = proxmox_virtual_environment_vm.vm["test"].initialization[0].user_account[0].username == "original" && proxmox_virtual_environment_vm.vm["test"].initialization[0].ip_config[0].ipv4[0].address == "192.0.2.2/24"
    error_message = "The lifecycle override must preserve the module's cloud-init ignore."
  }
}
