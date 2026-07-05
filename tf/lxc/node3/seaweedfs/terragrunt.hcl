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
      # Bumped from 4C/4GB: in an LXC the page cache counts against the memory
      # cgroup, so serving the multi-GB custom images (cloud-images bucket, e.g.
      # the xrdp desktop images) drove cgroup memory to the 4GB cap and the
      # kernel OOM-killed weed mid-download ("volume server has been killed" in
      # the journal). At idle the data-file page cache already sat at ~3.5GB.
      # 8GB gives cache+heap headroom; 4GB swap gives weed's Go heap (anonymous
      # memory) a reclaim target so cache eviction does not race the OOM killer.
      cores       = 4
      memory      = 8192
      swap        = 4096
      bridge      = local.common.locals.node3.net50.bridge
      ipv4        = "192.168.50.31/24"
      ipv4gw      = local.common.locals.node3.net50.ipv4gw
      dns_servers = local.common.locals.dns_internal
      disks = {
        disk0 = merge(local.env.locals.disk_defaults, {
          # Rootfs only. SeaweedFS object data lives on the dedicated
          # /var/lib/seaweedfs mount point below.
          size = 40
        })
      }
      mount_points = {
        data = {
          # Allocate a dedicated SeaweedFS data volume on the node3 USB SSD.
          # The mount hides any existing files at /var/lib/seaweedfs until data
          # is migrated into the new volume.
          volume = "usb-ssd"
          path   = "/var/lib/seaweedfs"
          size   = "100G"
          backup = false
        }
      }
    })
  }
}
