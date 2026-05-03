locals {
  container_defaults = {
    node_name    = "node2"
    unprivileged = true
    nesting      = true
    ifname       = "eth0"
    on_boot      = true
    os_template  = "local:vztmpl/ubuntu-24.04-standard_24.04-2_amd64.tar.zst"
    os_type      = "ubuntu"
  }
  disk_defaults = {
    datastore_id = "local-lvm"
  }
}
