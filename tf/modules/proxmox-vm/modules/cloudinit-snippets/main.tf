terraform {
  required_providers {
    proxmox = {
      source = "bpg/proxmox"
    }
  }
}

variable "node_name" {
  type = string
}

variable "datastore_id" {
  type = string
}

variable "snippets" {
  type = map(object({
    file_name = string
    content   = string
  }))
}

resource "proxmox_virtual_environment_file" "snippet" {
  for_each = var.snippets

  node_name    = var.node_name
  datastore_id = var.datastore_id
  content_type = "snippets"

  source_raw {
    file_name = each.value.file_name
    data      = each.value.content
  }
}

output "file_ids" {
  value = { for key, snippet in proxmox_virtual_environment_file.snippet : key => snippet.id }
}
