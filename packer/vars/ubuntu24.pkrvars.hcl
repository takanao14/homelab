iso_url      = "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
iso_checksum = "file:https://cloud-images.ubuntu.com/noble/current/SHA256SUMS"
ssh_username = "ubuntu"
distro       = "ubuntu"
provision_scripts = [
  "scripts/ubuntu/qemu-ga.sh",
]
cleanup_script = "scripts/ubuntu/cleanup.sh"
# 16G is the ceiling: tf/vm/node2/openbao and tf/vm/pve/sssdtest declare 16G
# disks, and Proxmox cannot import an image larger than the target disk.
disk_size = "16G"
