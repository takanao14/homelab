mock_provider "proxmox" {}
mock_provider "local" {
  mock_data "local_file" {
    defaults = { content = "ssh-ed25519 test" }
  }
}

variables {
  password       = "test-only"
  ssh_public_key = "/unused"
  containers = {
    recipe = {
      node_name    = "test"
      unprivileged = true
      nesting      = false
      ipv4         = "192.0.2.3/24"
      ipv4gw       = "192.0.2.1"
      bridge       = "vmbr0"
      ifname       = "eth0"
      dns_servers  = ["192.0.2.53"]
      os_template  = "local:vztmpl/debian-13-standard_13.6-1_amd64.tar.zst"
      os_type      = "debian"
      cores        = 1
      memory       = 512
      on_boot      = false
      disks        = { disk0 = { datastore_id = "local-zfs", size = 8 } }
    }
  }
}

run "creation_recipe_metadata" {
  command = plan
  variables {
    containers = merge({
      for name, guest in var.containers : name => merge(guest, { os_recipe_audit = true })
    }, { excluded = var.containers.recipe })
  }
  assert {
    condition = length(output.guest_os_recipes) == 1 && output.guest_os_recipes.recipe == {
      kind = "lxc", node = "test", image = "local:vztmpl/debian-13-standard_13.6-1_amd64.tar.zst"
    }
    error_message = "Export only placement and the creation template."
  }
}

run "excluded_by_default" {
  command = plan
  assert {
    condition     = output.guest_os_recipes == null
    error_message = "Guests must be excluded unless explicitly opted in."
  }
}
