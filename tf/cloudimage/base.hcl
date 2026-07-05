# Shared config for every node stack in this directory. Each node directory
# holds a minimal terragrunt.hcl (root + base includes) and a node.hcl
# declaring the Proxmox `node_name`. All stock images are deployed to every node.
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
