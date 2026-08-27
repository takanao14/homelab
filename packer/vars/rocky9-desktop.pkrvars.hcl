# GenericCloud includes qemu-guest-agent.
iso_url      = "https://download.rockylinux.org/pub/rocky/9/images/x86_64/Rocky-9-GenericCloud-Base.latest.x86_64.qcow2"
iso_checksum = "file:https://download.rockylinux.org/pub/rocky/9/images/x86_64/CHECKSUM"
ssh_username = "rocky"
distro       = "rocky"
provision_scripts = [
  "scripts/common/timezone.sh",
  "scripts/rocky/epel.sh",
  "scripts/rocky/xrdp.sh",
  "scripts/rocky/container.sh",
  "scripts/rocky/vm.sh",
  "scripts/rocky/tools.sh",
]
cleanup_script  = "scripts/rocky/cleanup.sh"
machine_profile = "desktop"
disk_size       = "20G"
