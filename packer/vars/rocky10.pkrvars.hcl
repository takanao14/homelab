# GenericCloud includes qemu-guest-agent; set the timezone and enable EPEL/CRB
# (the shared toolchain needs them) before installing.
iso_url      = "https://download.rockylinux.org/pub/rocky/10/images/x86_64/Rocky-10-GenericCloud-Base.latest.x86_64.qcow2"
iso_checksum = "file:https://download.rockylinux.org/pub/rocky/10/images/x86_64/CHECKSUM"
ssh_username = "rocky"
distro       = "rocky"
provision_scripts = [
  "scripts/rocky/timezone.sh",
  "scripts/rocky/epel.sh",
]
cleanup_script = "scripts/rocky/cleanup.sh"
disk_size      = "16G"
