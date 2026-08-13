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
    cores       = number
    memory      = number
    # Memory-cgroup swap in MB; default preserves the previous no-swap behavior.
    swap    = optional(number, 0)
    on_boot = bool
    disks = map(object({
      datastore_id = string
      size         = number
    }))
    mount_points = optional(map(object({
      volume        = string
      path          = string
      size          = optional(string)
      acl           = optional(bool)
      backup        = optional(bool, false)
      mount_options = optional(list(string))
      quota         = optional(bool)
      read_only     = optional(bool)
      replicate     = optional(bool)
      shared        = optional(bool)
    })), {})
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
