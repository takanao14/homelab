variable "images" {
  description = "Map of images to download. Keyed by image identifier."
  type = map(object({
    url          = string
    file_name    = string
    content_type = string
    node_name    = string
    datastore_id = string
  }))
  default = {}
}

resource "proxmox_virtual_environment_download_file" "image" {
  for_each = var.images

  url          = each.value.url
  file_name    = each.value.file_name
  content_type = each.value.content_type
  node_name    = each.value.node_name
  datastore_id = each.value.datastore_id
}
