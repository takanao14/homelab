locals {
  container_defaults = {
    node_name    = "node3"
    unprivileged = true
    nesting      = true
    ifname       = "eth0"
    on_boot      = true
  }
  disk_defaults = {
    datastore_id = "local-lvm"
  }
}
