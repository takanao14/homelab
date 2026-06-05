include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${get_parent_terragrunt_dir()}/modules/proxmox-image-upload"
}

locals {
  images_common = read_terragrunt_config(find_in_parent_folders("images.hcl"))
  images_dir    = "${get_parent_terragrunt_dir()}/../packer/images"
  node_name     = "pve"
  datastore_id  = "local"
  image_keys = [
    "ubuntu-24.04-custom",
    "ubuntu-24.04-xrdp",
    "rocky-9-xrdp",
    "rocky-10-custom",
    "debian-13-custom",
  ]
}

inputs = {
  images = {
    for name in local.image_keys : name => {
      file_name    = "${local.images_dir}/${local.images_common.locals.image_definitions[name].file_name}"
      content_type = local.images_common.locals.image_definitions[name].content_type
      node_name    = local.node_name
      datastore_id = local.datastore_id
    }
  }
}
