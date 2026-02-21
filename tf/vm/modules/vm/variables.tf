variable "vms" {
  description = "Map of VMs to create"
  type = map(object({
    node_name        = string
    config_datastore = string
    cores            = number
    memory           = number
    qemu_guest_agent = bool
    on_boot          = bool
    username         = string
    ipv4             = string
    ipv4gw           = string
    bridge           = string
    dns_servers      = list(string)
    scsi_hardware    = optional(string)
    disks = map(object({
      datastore_id = string
      size         = number
      file_id      = optional(string)
      cache        = optional(string)
      file_format  = optional(string)
      ssd          = optional(bool)
      discard      = optional(string)
    }))
  }))
}

variable "password" {
  description = "Password for the virtual machine"
  type        = string
  sensitive   = true
}

variable "ssh_public_key" {
  description = "Path to the SSH public key file"
  type        = string
}

