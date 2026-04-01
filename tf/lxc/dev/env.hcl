locals {
  container_defaults = {
    node_name    = "pve"
    unprivileged = true
    nesting      = true
    ifname       = "eth0"
    bridge       = "vnets001"
    ipv4gw       = "192.168.20.1"
    cores        = 2
    memory       = 1024
    on_boot      = true
    os_template  = "local:vztmpl/ubuntu-24.04-standard_24.04-2_amd64.tar.zst"
    os_type      = "ubuntu"
  }
  disk_defaults = {
    datastore_id = "local-zfs"
    size         = 10
  }
}
