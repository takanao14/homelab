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
      cores       = 2
      memory      = 2048
      bridge      = local.common.locals.node3.net50.bridge
      ipv4        = "192.168.50.31/24"
      ipv4gw      = local.common.locals.node3.net50.ipv4gw
      dns_servers = local.common.locals.dns_internal
      disks = {
        disk0 = merge(local.env.locals.disk_defaults, {
          # Sized to hold the cloud-images bucket (custom Packer images) in
          # addition to the tfstate backup. node3 local-lvm thin pool has ample
          # free space. Disk can only grow, never shrink.
          size = 100
        })
      }
    })
  }
}
