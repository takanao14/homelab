variable "containers" {
  description = "Map of VMs to create"
  type = map(object({
    node_name    = string
    unprivileged = bool
    nesting      = bool

    ipv4        = string
    ipv4gw      = string
    bridge      = string
    ifname      = string
    dns_servers = list(string)
    os_template = string
    os_type     = string
    cores            = number
    memory           = number
    on_boot          = bool
    disks = map(object({
      datastore_id = string
      size         = number
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

