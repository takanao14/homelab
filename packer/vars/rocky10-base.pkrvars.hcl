# GenericCloud includes qemu-guest-agent, so there is no qemu-ga.sh step.
iso_url      = "https://download.rockylinux.org/pub/rocky/10/images/x86_64/Rocky-10-GenericCloud-Base.latest.x86_64.qcow2"
iso_checksum = "file:https://download.rockylinux.org/pub/rocky/10/images/x86_64/CHECKSUM"
ssh_username = "rocky"
distro       = "rocky"
provision_scripts = [
  "scripts/common/timezone.sh",
]
cleanup_script    = "scripts/rocky/cleanup.sh"
install_toolchain = false
