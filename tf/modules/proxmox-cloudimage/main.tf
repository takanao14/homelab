resource "proxmox_virtual_environment_download_file" "image" {
  for_each = var.images

  url          = each.value.url
  file_name    = each.value.file_name
  content_type = each.value.content_type
  node_name    = each.value.node_name
  datastore_id = each.value.datastore_id
}
