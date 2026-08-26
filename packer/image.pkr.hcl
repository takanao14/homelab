# Shared image template; build.sh combines target vars and output settings.
# var.machine_profile selects the server (headless) or desktop (XRDP) role and
# is the only switch the shared toolchain needs: the install wrappers skip the
# desktop-only components on the server profile.
packer {
  required_plugins {
    qemu = {
      # renovate: datasource=github-releases depName=hashicorp/packer-plugin-qemu
      version = "1.1.6"
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

# Cloud-init SSH key; empty reads the builder key, while CI passes a validation stub.
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
  description = "Distro-specific provisioners, run in order (cleanup excluded)"
}

variable "cleanup_script" {
  type        = string
  description = "Cleanup script run last (purges caches, cloud-init data, build user)"
}

variable "machine_profile" {
  type        = string
  default     = "server"
  description = "Image role: server (headless) or desktop (adds GUI components)"

  validation {
    condition     = contains(["server", "desktop"], var.machine_profile)
    error_message = "The machine_profile variable must be server or desktop."
  }
}

variable "install_toolchain" {
  type        = bool
  default     = true
  description = "Install the shared CLI toolchain (false for unsupported distros)"
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

  # Persist the image role first: the installers below and provision.sh on the
  # running VM both read it, and cleanup later removes the build user.
  provisioner "shell" {
    inline = [
      "sudo install -d -m 0755 /etc/provisioning",
      "printf '%s\\n' '${var.machine_profile}' | sudo tee /etc/provisioning/machine-profile.local >/dev/null",
      "sudo chmod 0644 /etc/provisioning/machine-profile.local",
    ]
  }

  # Distro-specific setup: guest agent, timezone, EPEL, desktop/XRDP, GUI apps.
  provisioner "shell" {
    scripts         = var.provision_scripts
    execute_command = "chmod +x {{ .Path }}; sudo -S bash -c '{{ .Vars }} {{ .Path }}'"
  }

  # Upload the install wrappers with their vendored installers; the staged
  # VENDOR_DIR prevents runtime GitHub fetches.
  provisioner "file" {
    source      = "../scripts/install"
    destination = "/tmp"
  }

  # System-wide kitty defaults, installed by toolchain.sh on desktop images.
  provisioner "file" {
    source      = "files/kitty.conf"
    destination = "/tmp/kitty.conf"
  }

  # Install the shared toolchain into /usr/local so it survives the build user's
  # removal; the profile decides whether the desktop extras are included.
  provisioner "shell" {
    script          = "scripts/common/toolchain.sh"
    execute_command = "TOOL_MACHINE_PROFILE=${var.machine_profile} INSTALL_TOOLCHAIN=${var.install_toolchain} bash '{{ .Path }}'"
  }

  # Clean up last: purges caches, cloud-init data and the build user.
  provisioner "shell" {
    script          = var.cleanup_script
    execute_command = "chmod +x {{ .Path }}; sudo -S bash -c '{{ .Vars }} {{ .Path }}'"
  }

  post-processor "shell-local" {
    inline = [
      "virt-sparsify --compress ${var.output_directory}/${var.vm_name} ${var.image_name}",
    ]
  }
}
