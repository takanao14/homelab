# Shared custom-image config; node.hcl selects the node and image keys.
terraform {
  source = "${dirname(find_in_parent_folders("root.hcl"))}/modules/proxmox-cloudimage"

  # Serialize large downloads to protect the Proxmox/S3 path.
  extra_arguments "serial_download" {
    commands  = ["apply", "destroy"]
    arguments = ["-parallelism=1"]
  }
}

locals {
  images_common = read_terragrunt_config(find_in_parent_folders("images.hcl"))
  node          = read_terragrunt_config("${get_terragrunt_dir()}/node.hcl")

  base_url     = local.images_common.locals.base_url
  datastore_id = "local"
}

inputs = {
  images = {
    for name in local.node.locals.image_keys : name => {
      url          = "${local.base_url}/${local.images_common.locals.image_definitions[name].file_name}"
      file_name    = local.images_common.locals.image_definitions[name].file_name
      content_type = local.images_common.locals.image_definitions[name].content_type
      # Detect same-URL rebuilds; trim the checksum without a shell-specific pipeline.
      checksum            = trimspace(run_cmd("--terragrunt-quiet", "curl", "-fsS", "${local.base_url}/${local.images_common.locals.image_definitions[name].file_name}.sha256"))
      node_name           = local.node.locals.node_name
      datastore_id        = local.datastore_id
      overwrite_unmanaged = true
    }
  }
}
