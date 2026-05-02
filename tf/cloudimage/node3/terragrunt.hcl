include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/proxmox-cloudimage"
}

locals {
  images_common = read_terragrunt_config(find_in_parent_folders("images.hcl"))
  node_name     = "node3"
  datastore_id  = "local"
}

inputs = {
  images = {
    for name, def in local.images_common.locals.image_definitions : name => merge(def, {
      node_name    = local.node_name
      datastore_id = local.datastore_id
    })
  }
}
