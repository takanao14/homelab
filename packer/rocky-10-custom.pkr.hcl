packer {
  required_plugins {
    qemu = {
      version = ">= 1.1.4"
      source  = "github.com/hashicorp/qemu"
    }
  }
}

# Variables for output configuration
variable "output_directory" {
  type        = string
  description = "Directory where the built image will be stored"
}

variable "vm_name" {
  type        = string
  description = "Name of the output VM image file"
}

variable "image_name" {
  type        = string
  description = "Name of the final image file after compression"
}

variable "user_password" {
  type        = string
  sensitive   = true
  description = "Password for the default user account (used in Cloud-Init)"
}

# ssh_pubkey is used in Cloud-Init user-data to set up SSH access for the default user account
locals {
  ssh_pubkey = file("~/.ssh/id_ed25519.pub")
}

source "qemu" "rocky10_custom" {
  # Official image URL and checksum
  iso_url      = "https://download.rockylinux.org/pub/rocky/10/images/x86_64/Rocky-10-GenericCloud-Base.latest.x86_64.qcow2"
  iso_checksum = "file:https://download.rockylinux.org/pub/rocky/10/images/x86_64/CHECKSUM"
  disk_image   = true

  cpus      = 2
  memory    = 2048
  cpu_model = "host"

  # Output settings
  output_directory = var.output_directory
  vm_name          = var.vm_name
  format           = "qcow2"
  disk_size        = "10G"
  accelerator      = "kvm"

  # SSH connection settings
  ssh_username   = "rocky"
  ssh_agent_auth = true
  ssh_timeout    = "15m"

  # Attach Cloud-Init as a seed disk
  cd_content = {
    "/user-data" = templatefile("./cinit/rocky/user-data.pkrtpl.hcl", {
      ssh_pubkey    = local.ssh_pubkey
      user_password = var.user_password
    }),
    "/meta-data" = file("./cinit/rocky/meta-data")
  }
  cd_label = "cidata"

  # Run headless (no display)
  headless = true
}

build {
  sources = ["source.qemu.rocky10_custom"]

  # Install packages and clean up
  provisioner "shell" {
    scripts = [
      "scripts/rocky/timezone.sh",
      "scripts/rocky/cleanup.sh"
    ]
    execute_command = "chmod +x {{ .Path }}; sudo -S bash -c '{{ .Vars }} {{ .Path }}'"
  }

  post-processor "shell-local" {
    inline = [
      "virt-sparsify --compress ${var.output_directory}/${var.vm_name} ${var.image_name}",
    ]
  }
}
