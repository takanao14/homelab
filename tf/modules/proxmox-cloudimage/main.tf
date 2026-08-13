resource "proxmox_download_file" "image" {
  for_each = var.images

  url                 = each.value.url
  file_name           = each.value.file_name
  content_type        = each.value.content_type
  node_name           = each.value.node_name
  datastore_id        = each.value.datastore_id
  overwrite           = true
  overwrite_unmanaged = each.value.overwrite_unmanaged

  # A changed checksum re-downloads same-URL rebuilds; set the algorithm only with it.
  checksum           = each.value.checksum
  checksum_algorithm = each.value.checksum != null ? each.value.checksum_algorithm : null
}
