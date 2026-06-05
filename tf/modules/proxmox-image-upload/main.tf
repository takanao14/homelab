resource "proxmox_virtual_environment_file" "image" {
  for_each = var.images

  source_file {
    path = each.value.file_name
  }
  content_type = each.value.content_type
  node_name    = each.value.node_name
  datastore_id = each.value.datastore_id
}
