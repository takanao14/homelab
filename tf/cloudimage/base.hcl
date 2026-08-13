# Shared stock-image config; node.hcl selects the Proxmox node.
terraform {
  source = "${dirname(find_in_parent_folders("root.hcl"))}/modules/proxmox-cloudimage"
}

locals {
  images_common = read_terragrunt_config(find_in_parent_folders("images.hcl"))
  node          = read_terragrunt_config("${get_terragrunt_dir()}/node.hcl")

  datastore_id = "local"
}

inputs = {
  images = {
    for name, def in local.images_common.locals.image_definitions : name => merge(def, {
      node_name    = local.node.locals.node_name
      datastore_id = local.datastore_id
    })
  }
}
