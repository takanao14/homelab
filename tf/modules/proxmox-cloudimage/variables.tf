variable "images" {
  description = "Map of images to download. Keyed by image identifier."
  type = map(object({
    url                 = string
    file_name           = string
    content_type        = string
    node_name           = string
    datastore_id        = string
    overwrite_unmanaged = optional(bool)
    # Custom images provide a sidecar SHA-256; stock images omit it.
    checksum           = optional(string)
    checksum_algorithm = optional(string, "sha256")
  }))
  default = {}
}
