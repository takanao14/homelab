include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/proxmox-container"
}

locals {
  env    = read_terragrunt_config(find_in_parent_folders("env.hcl"))
  common = read_terragrunt_config(find_in_parent_folders("common.hcl"))
}

inputs = {
  containers = {
    "seaweedfs1" = merge(local.env.locals.container_defaults, {
      # 8 GB RAM accommodates the ~3.5 GB idle page cache; 4 GB swap protects
      # the Go heap from OOM kills while serving multi-GB images.
      cores       = 4
      memory      = 8192
      swap        = 4096
      bridge      = local.common.locals.node3.net50.bridge
      ipv4        = "192.168.50.31/24"
      ipv4gw      = local.common.locals.node3.net50.ipv4gw
      dns_servers = local.common.locals.dns_internal
      disks = {
        disk0 = merge(local.env.locals.disk_defaults, {
          # Rootfs only; object data uses the mount below.
          size = 40
        })
      }
      mount_points = {
        data = {
          # Dedicated object-data volume on the node3 USB SSD.
          volume = "usb-ssd"
          path   = "/var/lib/seaweedfs"
          size   = "200G"
          backup = false
        }
      }
    })
  }
}
