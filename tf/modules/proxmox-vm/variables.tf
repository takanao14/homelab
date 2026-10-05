variable "vms" {
  description = "Map of VMs to create"
  type = map(object({
    node_name        = string
    config_datastore = string
    config_interface = optional(string)
    cores            = number
    memory           = number
    qemu_guest_agent = bool
    on_boot          = bool
    started          = optional(bool, true)
    username         = optional(string)
    ipv4             = optional(string)
    ipv4gw           = optional(string)
    bridge           = string
    dns_domain       = optional(string)
    dns_servers      = optional(list(string), [])
    os_type          = optional(string)
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
    balloon = optional(bool, false)
    cloud_init = optional(object({
      type              = optional(string)
      snippet_datastore = optional(string, "local")
      user_data         = optional(string)
      network_data      = optional(string)
    }), {})
    pci_devices = optional(map(object({
      id      = optional(string)
      mapping = optional(string)
      pcie    = optional(bool, true)
      rombar  = optional(bool, true)
    })), {})
  }))

  validation {
    condition = alltrue([for vm in values(var.vms) :
      vm.cloud_init.user_data != null ? trimspace(vm.cloud_init.user_data) != "" : try(trimspace(vm.username) != "", false)
    ])
    error_message = "Each VM requires a non-empty username or custom user_data."
  }

  validation {
    condition = alltrue([for vm in values(var.vms) :
      vm.cloud_init.network_data != null ? trimspace(vm.cloud_init.network_data) != "" : try(trimspace(vm.ipv4) != "" && (vm.ipv4 == "dhcp" ? true : try(trimspace(vm.ipv4gw) != "", false)), false)
    ])
    error_message = "Each VM requires non-empty network_data or an IPv4 address and gateway (except DHCP)."
  }

  validation {
    condition = alltrue([for vm in values(var.vms) :
      vm.cloud_init.type == null ? true : contains(["nocloud", "configdrive2", "opennebula"], vm.cloud_init.type)
    ])
    error_message = "cloud_init.type must be nocloud, configdrive2, or opennebula."
  }
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
