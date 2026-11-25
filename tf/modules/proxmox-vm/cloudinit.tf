locals {
  network_data = {
    for name, vm in var.vms : name => (
      vm.cloud_init.network_data != null ? vm.cloud_init.network_data :
      vm.cloud_init.netplan_virtio ? yamlencode({
        version  = 2
        renderer = "networkd"
        ethernets = {
          primary = {
            # The VM has one VirtIO NIC; its PCI-derived name can change.
            match     = { driver = "virtio_net" }
            addresses = [vm.ipv4]
            routes = [{
              to  = "default"
              via = vm.ipv4gw
            }]
            nameservers = {
              addresses = vm.dns_servers
              search    = vm.dns_domain != null ? [vm.dns_domain] : []
            }
          }
        }
      }) : null
    )
  }
}

module "cloudinit_snippets" {
  source = "./modules/cloudinit-snippets"
  for_each = {
    for name, vm in var.vms : name => vm
    if vm.cloud_init.user_data != null || local.network_data[name] != null
  }

  providers = {
    proxmox = proxmox.snippets
  }

  node_name    = each.value.node_name
  datastore_id = each.value.cloud_init.snippet_datastore
  snippets = {
    for kind, data in {
      user    = each.value.cloud_init.user_data
      network = local.network_data[each.key]
      } : kind => {
      file_name = "${each.key}-${kind}.yaml"
      content   = data
    } if data != null
  }
}
