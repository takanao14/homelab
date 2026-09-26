include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/proxmox-container"
}

locals {
  env        = read_terragrunt_config(find_in_parent_folders("env.hcl"))
  common     = read_terragrunt_config(find_in_parent_folders("common.hcl"))
  dns_images = read_terragrunt_config("${get_parent_terragrunt_dir()}/lxc/dns-images.hcl")
}

inputs = {
  containers = {
    "ns1" = merge(local.env.locals.container_defaults, {
      os_template = local.dns_images.locals.templates[local.dns_images.locals.releases.ns1]
      cores       = 1
      memory      = 1024
      bridge      = local.common.locals.node2.net10.bridge
      ipv4        = "192.168.10.233/24"
      ipv4gw      = local.common.locals.node2.net10.ipv4gw
      dns_servers = local.common.locals.dns_external
      disks = {
        disk0 = merge(local.env.locals.disk_defaults, {
          size = 4
        })
      }
    })
    "dist1" = merge(local.env.locals.container_defaults, {
      os_template = local.dns_images.locals.templates[local.dns_images.locals.releases.dist1]
      cores       = 2
      memory      = 1024
      bridge      = local.common.locals.node2.net10.bridge
      ipv4        = "192.168.10.231/24"
      ipv4gw      = local.common.locals.node2.net10.ipv4gw
      dns_servers = local.common.locals.dns_external
      disks = {
        disk0 = merge(local.env.locals.disk_defaults, {
          size = 4
        })
      }
    })
    "ns3" = merge(local.env.locals.container_defaults, {
      os_template = local.dns_images.locals.templates[local.dns_images.locals.releases.ns3]
      cores       = 1
      memory      = 1024
      bridge      = local.common.locals.node2.net10.bridge
      ipv4        = "192.168.10.235/24"
      ipv4gw      = local.common.locals.node2.net10.ipv4gw
      dns_servers = local.common.locals.dns_external
      disks = {
        disk0 = merge(local.env.locals.disk_defaults, {
          size = 4
        })
      }
    })
  }
}
