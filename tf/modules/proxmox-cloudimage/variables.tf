variable "images" {
  description = "Map of images to download. Keyed by image identifier."
  type = map(object({
    url                 = string
    file_name           = string
    content_type        = string
    node_name           = string
    datastore_id        = string
    overwrite_unmanaged = optional(bool)
    # Optional integrity check. When set, a changing digest re-triggers the
    # download. Custom images pass the sha256 published next to the object (see
    # tf/customimage); stock images omit it.
    checksum           = optional(string)
    checksum_algorithm = optional(string, "sha256")
  }))
  default = {}
}
