# Shared template for the XRDP desktop images. Per-distro inputs live in
# vars/<target>-xrdp.pkrvars.hcl; build.sh selects the var file and injects the
# output variables. The headless server variant is basic.pkr.hcl.
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

# SSH public key injected into cloud-init user-data for the default user.
# Empty (default) means "read the builder's ~/.ssh/id_ed25519.pub"; CI passes
# a stub value so validate needs no key file on the runner.
variable "ssh_pubkey" {
  type        = string
  default     = ""
  description = "SSH public key for the default user (empty = read ~/.ssh/id_ed25519.pub)"
}

# --- Distro configuration (vars/<target>-xrdp.pkrvars.hcl) ---

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
  description = "Shell provisioner scripts for base setup, desktop/XRDP and tooling"
}

variable "cleanup_script" {
  type        = string
  description = "Cleanup script run last (purges caches, cloud-init data, build user)"
}

variable "disk_size" {
  type        = string
  default     = "20G"
  description = "Disk size of the built image"
}

locals {
  ssh_pubkey = var.ssh_pubkey != "" ? var.ssh_pubkey : file("~/.ssh/id_ed25519.pub")
}

source "qemu" "xrdp" {
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
  sources = ["source.qemu.xrdp"]

  # Install base packages: guest agent / timezone, desktop/XRDP, container
  # runtime, virtualization and GUI tools (Firefox, VS Code, Wireshark,
  # HashiCorp).
  provisioner "shell" {
    scripts         = var.provision_scripts
    execute_command = "chmod +x {{ .Path }}; sudo -S bash -c '{{ .Vars }} {{ .Path }}'"
  }

  # Upload the vendored dotfiles installers next to where the wrappers run, so
  # install-*.sh use local copies instead of fetching them from GitHub during
  # the build. VENDOR_DIR points each wrapper at this directory.
  provisioner "file" {
    source      = "../scripts/install/vendor"
    destination = "/tmp"
  }

  # Install the system-package prerequisites before the unprivileged tool and
  # font installers consume them.
  provisioner "shell" {
    script          = "../scripts/install/packages.sh"
    execute_command = "VENDOR_DIR=/tmp/vendor bash '{{ .Path }}' global"
  }

  # Bake the CLI toolchain (kubectl, helm, terragrunt, opentofu, k9s, …)
  # system-wide via the shared homelab wrapper -- the single source of truth
  # also used by scripts/provision.sh. Global mode self-elevates with sudo and
  # installs into /usr/local/bin.
  provisioner "shell" {
    script          = "../scripts/install/tools.sh"
    execute_command = "VENDOR_DIR=/tmp/vendor bash '{{ .Path }}' global"
  }

  # Bake the UDEV Gothic NF font system-wide via the shared homelab wrapper.
  # TOOL_FORCE_GUI_INSTALL=1 skips the live-GUI check (xrdp is not running yet
  # during the build).
  provisioner "shell" {
    script          = "../scripts/install/fonts.sh"
    execute_command = "TOOL_FORCE_GUI_INSTALL=1 VENDOR_DIR=/tmp/vendor bash '{{ .Path }}' global"
  }

  # Install the kitty terminal system-wide (into /usr/local/kitty.app).
  provisioner "shell" {
    script          = "../scripts/install/terminal.sh"
    execute_command = "TOOL_FORCE_GUI_INSTALL=1 VENDOR_DIR=/tmp/vendor bash '{{ .Path }}' global"
  }

  # Default kitty config for all users (UDEV Gothic font); kitty reads
  # /etc/xdg/kitty/kitty.conf via XDG_CONFIG_DIRS.
  provisioner "file" {
    source      = "files/kitty.conf"
    destination = "/tmp/kitty.conf"
  }
  provisioner "shell" {
    inline = [
      "sudo install -D -m 0644 /tmp/kitty.conf /etc/xdg/kitty/kitty.conf",
      "rm -f /tmp/kitty.conf",
    ]
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
