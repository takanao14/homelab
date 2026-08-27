# GenericCloud includes qemu-guest-agent, so there is no qemu-ga.sh step.
# EPEL/CRB are enabled ahead of the shared toolchain, which needs them.
iso_url      = "https://download.rockylinux.org/pub/rocky/9/images/x86_64/Rocky-9-GenericCloud-Base.latest.x86_64.qcow2"
iso_checksum = "file:https://download.rockylinux.org/pub/rocky/9/images/x86_64/CHECKSUM"
ssh_username = "rocky"
distro       = "rocky"
provision_scripts = [
  "scripts/common/timezone.sh",
  "scripts/rocky/epel.sh",
]
cleanup_script = "scripts/rocky/cleanup.sh"
disk_size      = "16G"
