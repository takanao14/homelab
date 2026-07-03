iso_url      = "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
iso_checksum = "file:https://cloud-images.ubuntu.com/noble/current/SHA256SUMS"
ssh_username = "ubuntu"
distro       = "ubuntu"
provision_scripts = [
  "scripts/ubuntu/qemu-ga.sh",
  "scripts/ubuntu/xrdp.sh",
  "scripts/ubuntu/container.sh",
  "scripts/ubuntu/vm.sh",
  "scripts/ubuntu/tools.sh",
]
cleanup_script = "scripts/ubuntu/cleanup.sh"
