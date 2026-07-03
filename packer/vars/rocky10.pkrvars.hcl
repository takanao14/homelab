# Rocky GenericCloud images ship qemu-guest-agent preinstalled, so only the
# timezone needs setting before cleanup.
iso_url      = "https://download.rockylinux.org/pub/rocky/10/images/x86_64/Rocky-10-GenericCloud-Base.latest.x86_64.qcow2"
iso_checksum = "file:https://download.rockylinux.org/pub/rocky/10/images/x86_64/CHECKSUM"
ssh_username = "rocky"
distro       = "rocky"
provision_scripts = [
  "scripts/rocky/timezone.sh",
  "scripts/rocky/cleanup.sh",
]
