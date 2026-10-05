module "cloudinit_snippets" {
  source = "./modules/cloudinit-snippets"
  for_each = {
    for name, vm in var.vms : name => vm
    if vm.cloud_init.user_data != null || vm.cloud_init.network_data != null
  }

  providers = {
    proxmox = proxmox.snippets
  }

  node_name    = each.value.node_name
  datastore_id = each.value.cloud_init.snippet_datastore
  snippets = {
    for kind, data in {
      user    = each.value.cloud_init.user_data
      network = each.value.cloud_init.network_data
      } : kind => {
      file_name = "${each.key}-${kind}.yaml"
      content   = data
    } if data != null
  }
}
