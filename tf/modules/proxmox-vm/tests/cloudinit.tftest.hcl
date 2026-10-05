mock_provider "proxmox" {}
mock_provider "proxmox" {
  alias = "snippets"
  mock_resource "proxmox_virtual_environment_file" {
    defaults = {
      id = "local:snippets/test.yaml"
    }
  }
}
mock_provider "local" {
  mock_data "local_file" {
    defaults = {
      content = "ssh-ed25519 test"
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
      cores            = 2
      memory           = 1024
      qemu_guest_agent = false
      on_boot          = false
      username         = "test"
      ipv4             = "192.0.2.2/24"
      ipv4gw           = "192.0.2.1"
      bridge           = "vmbr0"
      os_type          = "other"
      disks            = {}
    }
  }
}

run "native" {
  command = plan
  assert {
    condition     = length(module.cloudinit_snippets) == 0 && length(proxmox_virtual_environment_vm.vm["test"].initialization[0].user_account) == 1 && length(proxmox_virtual_environment_vm.vm["test"].initialization[0].ip_config) == 1
    error_message = "Native initialization must retain user and IP blocks without snippets."
  }
}

run "user_only" {
  command = plan
  variables {
    vms = {
      test = {
        node_name        = "test"
        config_datastore = "local-zfs"
        cores            = 2
        memory           = 1024
        qemu_guest_agent = false
        on_boot          = false
        ipv4             = "192.0.2.2/24"
        ipv4gw           = "192.0.2.1"
        bridge           = "vmbr0"
        os_type          = "other"
        disks            = {}
        cloud_init = {
          user_data = "#cloud-config\nusers: []\n"
        }
      }
    }
  }
  assert {
    condition     = toset(keys(module.cloudinit_snippets["test"].file_ids)) == toset(["user"]) && length(proxmox_virtual_environment_vm.vm["test"].initialization[0].user_account) == 0 && length(proxmox_virtual_environment_vm.vm["test"].initialization[0].ip_config) == 1 && length(proxmox_virtual_environment_vm.vm["test"].initialization[0].dns) == 1
    error_message = "Custom payloads must suppress only their corresponding native blocks."
  }
}

run "network_only" {
  command = plan
  variables {
    vms = {
      test = {
        node_name        = "test"
        config_datastore = "local-zfs"
        cores            = 2
        memory           = 1024
        qemu_guest_agent = false
        on_boot          = false
        username         = "test"
        bridge           = "vmbr0"
        os_type          = "other"
        disks            = {}
        cloud_init = {
          network_data = "version: 2\nethernets: {}\n"
        }
      }
    }
  }
  assert {
    condition     = toset(keys(module.cloudinit_snippets["test"].file_ids)) == toset(["network"]) && length(proxmox_virtual_environment_vm.vm["test"].initialization[0].user_account) == 1 && length(proxmox_virtual_environment_vm.vm["test"].initialization[0].ip_config) == 0 && length(proxmox_virtual_environment_vm.vm["test"].initialization[0].dns) == 0
    error_message = "Custom payloads must suppress only their corresponding native blocks."
  }
}

run "both" {
  command = plan
  variables {
    vms = {
      test = {
        node_name        = "test"
        config_datastore = "local-zfs"
        cores            = 2
        memory           = 1024
        qemu_guest_agent = false
        on_boot          = false
        bridge           = "vmbr0"
        os_type          = "other"
        disks            = {}
        cloud_init = {
          user_data    = "#cloud-config\nusers: []\n"
          network_data = "version: 2\nethernets: {}\n"
        }
      }
    }
  }
  assert {
    condition     = toset(keys(module.cloudinit_snippets["test"].file_ids)) == toset(["user", "network"]) && length(proxmox_virtual_environment_vm.vm["test"].initialization[0].user_account) == 0 && length(proxmox_virtual_environment_vm.vm["test"].initialization[0].ip_config) == 0 && length(proxmox_virtual_environment_vm.vm["test"].initialization[0].dns) == 0
    error_message = "Custom payloads must suppress only their corresponding native blocks."
  }
}

run "empty_payload" {
  command = plan
  variables {
    vms = {
      test = {
        node_name        = "test"
        config_datastore = "local-zfs"
        cores            = 2
        memory           = 1024
        qemu_guest_agent = false
        on_boot          = false
        username         = "test"
        ipv4             = "192.0.2.2/24"
        ipv4gw           = "192.0.2.1"
        bridge           = "vmbr0"
        os_type          = "other"
        disks            = {}
        cloud_init       = { user_data = " " }
      }
    }
  }
  expect_failures = [var.vms]
}

run "missing_username" {
  command = plan
  variables {
    vms = {
      test = {
        node_name        = "test"
        config_datastore = "local-zfs"
        cores            = 2
        memory           = 1024
        qemu_guest_agent = false
        on_boot          = false
        ipv4             = "192.0.2.2/24"
        ipv4gw           = "192.0.2.1"
        bridge           = "vmbr0"
        os_type          = "other"
        disks            = {}
      }
    }
  }
  expect_failures = [var.vms]
}

run "missing_gateway" {
  command = plan
  variables {
    vms = {
      test = {
        node_name        = "test"
        config_datastore = "local-zfs"
        cores            = 2
        memory           = 1024
        qemu_guest_agent = false
        on_boot          = false
        username         = "test"
        ipv4             = "192.0.2.2/24"
        bridge           = "vmbr0"
        os_type          = "other"
        disks            = {}
      }
    }
  }
  expect_failures = [var.vms]
}

run "invalid_type" {
  command = plan
  variables {
    vms = {
      test = {
        node_name        = "test"
        config_datastore = "local-zfs"
        cores            = 2
        memory           = 1024
        qemu_guest_agent = false
        on_boot          = false
        username         = "test"
        ipv4             = "192.0.2.2/24"
        ipv4gw           = "192.0.2.1"
        bridge           = "vmbr0"
        os_type          = "other"
        disks            = {}
        cloud_init       = { type = "invalid" }
      }
    }
  }
  expect_failures = [var.vms]
}
