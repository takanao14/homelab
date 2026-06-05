variable "images" {
  description = "Map of locally built images to upload to a Proxmox datastore. Keyed by image identifier."
  type = map(object({
    file_name    = string
    content_type = string
    node_name    = string
    datastore_id = string
  }))
  default = {}
}
