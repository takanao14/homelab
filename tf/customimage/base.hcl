# Shared config for every node stack in this directory. Each node directory
# holds a minimal terragrunt.hcl (root + base includes) and a node.hcl
# declaring `node_name` and the `image_keys` deployed to that node.
terraform {
  source = "${get_parent_terragrunt_dir("root")}/modules/proxmox-cloudimage"

  # Image downloads are large; running them in parallel overwhelms the Proxmox
  # node / S3 path and times out. Force serial downloads.
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
      # Pin the sha256 published next to the object so a rebuilt image (same URL,
      # new content) is re-downloaded. Fails fast if the image is not yet pushed.
      checksum            = run_cmd("--terragrunt-quiet", "sh", "-c", "curl -fsS '${local.base_url}/${local.images_common.locals.image_definitions[name].file_name}.sha256' | tr -d '[:space:]'")
      node_name           = local.node.locals.node_name
      datastore_id        = local.datastore_id
      overwrite_unmanaged = true
    }
  }
}
