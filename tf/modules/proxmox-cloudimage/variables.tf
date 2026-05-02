variable "images" {
  description = "Map of images to download. Keyed by image identifier."
  type = map(object({
    url                 = string
    file_name           = string
    content_type        = string
    node_name           = string
    datastore_id        = string
    overwrite_unmanaged = optional(bool)
  }))
  default = {}
}
