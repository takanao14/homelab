resource "proxmox_download_file" "image" {
  for_each = var.images

  url                 = each.value.url
  file_name           = each.value.file_name
  content_type        = each.value.content_type
  node_name           = each.value.node_name
  datastore_id        = each.value.datastore_id
  overwrite           = true
  overwrite_unmanaged = each.value.overwrite_unmanaged

  # Optional integrity check. A changing checksum forces Proxmox to re-download
  # even when the URL is unchanged, so rebuilt custom images are detected.
  # checksum_algorithm must only be set when a checksum is present.
  checksum           = each.value.checksum
  checksum_algorithm = each.value.checksum != null ? each.value.checksum_algorithm : null
}
