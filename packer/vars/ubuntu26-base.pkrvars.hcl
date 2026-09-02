iso_url      = "https://cloud-images.ubuntu.com/resolute/current/resolute-server-cloudimg-amd64.img"
iso_checksum = "file:https://cloud-images.ubuntu.com/resolute/current/SHA256SUMS"
ssh_username = "ubuntu"
distro       = "ubuntu"
provision_scripts = [
  "scripts/ubuntu/qemu-ga.sh",
  "scripts/common/timezone.sh",
]
cleanup_script    = "scripts/ubuntu/cleanup.sh"
install_toolchain = false
