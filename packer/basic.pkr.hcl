# Shared template for the basic (headless server) images. Per-distro inputs
# live in vars/<target>.pkrvars.hcl; build.sh selects the var file and injects
# the output variables. The XRDP desktop variant is xrdp.pkr.hcl.
packer {
  required_plugins {
    qemu = {
      version = ">= 1.1.4"
      source  = "github.com/hashicorp/qemu"
    }
  }
}

# --- Output configuration (injected by build.sh) ---

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

# SSH public key injected into cloud-init user-data for the default user.
# Empty (default) means "read the builder's ~/.ssh/id_ed25519.pub"; CI passes
# a stub value so validate needs no key file on the runner.
variable "ssh_pubkey" {
  type        = string
  default     = ""
  description = "SSH public key for the default user (empty = read ~/.ssh/id_ed25519.pub)"
}

# --- Distro configuration (vars/<target>.pkrvars.hcl) ---

variable "iso_url" {
  type        = string
  description = "Upstream cloud image URL"
}

variable "iso_checksum" {
  type        = string
  description = "Upstream image checksum (file:<url> form)"
}

variable "ssh_username" {
  type        = string
  description = "Default user of the upstream cloud image"
}

variable "distro" {
  type        = string
  description = "cloud-init template directory under cinit/"
}

variable "provision_scripts" {
  type        = list(string)
  description = "Shell provisioner scripts, run in order (cleanup last)"
}

variable "disk_size" {
  type        = string
  default     = "10G"
  description = "Disk size of the built image"
}

locals {
  ssh_pubkey = var.ssh_pubkey != "" ? var.ssh_pubkey : file("~/.ssh/id_ed25519.pub")
}

source "qemu" "custom" {
  iso_url      = var.iso_url
  iso_checksum = var.iso_checksum
  disk_image   = true

  cpus      = 2
  memory    = 2048
  cpu_model = "host"

  # Output settings
  output_directory = var.output_directory
  vm_name          = var.vm_name
  format           = "qcow2"
  disk_size        = var.disk_size
  accelerator      = "kvm"

  # SSH connection settings
  ssh_username   = var.ssh_username
  ssh_agent_auth = true
  ssh_timeout    = "15m"

  # Attach Cloud-Init as a seed disk
  cd_content = {
    "/user-data" = templatefile("./cinit/${var.distro}/user-data.pkrtpl.hcl", {
      ssh_pubkey    = local.ssh_pubkey
      user_password = var.user_password
    }),
    "/meta-data" = file("./cinit/${var.distro}/meta-data")
  }
  cd_label = "cidata"

  # Run headless (no display)
  headless = true
}

build {
  sources = ["source.qemu.custom"]

  # Install packages and clean up
  provisioner "shell" {
    scripts         = var.provision_scripts
    execute_command = "chmod +x {{ .Path }}; sudo -S bash -c '{{ .Vars }} {{ .Path }}'"
  }

  post-processor "shell-local" {
    inline = [
      "virt-sparsify --compress ${var.output_directory}/${var.vm_name} ${var.image_name}",
    ]
  }
}
