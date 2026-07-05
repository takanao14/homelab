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
    # Swap (MB) backing the container's memory cgroup. Defaults to 0 (no swap),
    # matching the previous behavior for all existing containers.
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
