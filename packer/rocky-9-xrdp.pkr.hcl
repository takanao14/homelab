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

source "qemu" "rocky_9_xrdp" {
  # Official image URL and checksum
  iso_url      = "https://download.rockylinux.org/pub/rocky/9/images/x86_64/Rocky-9-GenericCloud-Base.latest.x86_64.qcow2"
  iso_checksum = "file:https://download.rockylinux.org/pub/rocky/9/images/x86_64/CHECKSUM"
  disk_image   = true

  cpus      = 2
  memory    = 2048
  cpu_model = "host"

  # Output settings
  output_directory = var.output_directory
  vm_name          = var.vm_name
  format           = "qcow2"
  disk_size        = "20G"
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
  sources = ["source.qemu.rocky_9_xrdp"]

  # Install base packages: timezone, desktop/XRDP, container runtime,
  # virtualization and GUI tools (Chrome, VS Code, Wireshark, HashiCorp).
  provisioner "shell" {
    scripts = [
      "scripts/rocky/timezone.sh",
      "scripts/rocky/xrdp.sh",
      "scripts/rocky/container.sh",
      "scripts/rocky/vm.sh",
      "scripts/rocky/tools.sh"
    ]
    execute_command = "chmod +x {{ .Path }}; sudo -S bash -c '{{ .Vars }} {{ .Path }}'"
  }

  # Upload the vendored dotfiles installers next to where the wrappers run, so
  # install-*.sh use local copies instead of fetching them from GitHub during
  # the build. VENDOR_DIR points each wrapper at this directory.
  provisioner "file" {
    source      = "../scripts/install/vendor"
    destination = "/tmp"
  }

  # Bake the CLI toolchain (kubectl, helm, terragrunt, opentofu, k9s, …)
  # system-wide via the shared homelab wrapper -- the single source of truth
  # also used by scripts/provision.sh. Global mode self-elevates with sudo and
  # installs into /usr/local/bin.
  provisioner "shell" {
    script          = "../scripts/install/install-tools.sh"
    execute_command = "VENDOR_DIR=/tmp/vendor bash '{{ .Path }}' global"
  }

  # Bake the UDEV Gothic NF font system-wide via the shared homelab wrapper.
  # TOOL_FORCE_GUI_INSTALL=1 skips the live-GUI check (xrdp is not running yet
  # during the build).
  provisioner "shell" {
    script          = "../scripts/install/install-fonts.sh"
    execute_command = "TOOL_FORCE_GUI_INSTALL=1 VENDOR_DIR=/tmp/vendor bash '{{ .Path }}' global"
  }

  # Install the kitty terminal system-wide (into /usr/local/kitty.app).
  provisioner "shell" {
    script          = "../scripts/install/install-terminal.sh"
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
    script          = "scripts/rocky/cleanup.sh"
    execute_command = "chmod +x {{ .Path }}; sudo -S bash -c '{{ .Vars }} {{ .Path }}'"
  }

  post-processor "shell-local" {
    inline = [
      "virt-sparsify --compress ${var.output_directory}/${var.vm_name} ${var.image_name}",
    ]
  }
}
