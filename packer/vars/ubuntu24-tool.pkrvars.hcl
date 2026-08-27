iso_url      = "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
iso_checksum = "file:https://cloud-images.ubuntu.com/noble/current/SHA256SUMS"
ssh_username = "ubuntu"
distro       = "ubuntu"
provision_scripts = [
  "scripts/ubuntu/qemu-ga.sh",
  "scripts/common/timezone.sh",
]
cleanup_script = "scripts/ubuntu/cleanup.sh"
disk_size      = "16G"
