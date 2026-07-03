iso_url      = "https://cloud.debian.org/images/cloud/trixie/latest/debian-13-genericcloud-amd64.qcow2"
iso_checksum = "file:https://cloud.debian.org/images/cloud/trixie/latest/SHA512SUMS"
ssh_username = "debian"
distro       = "debian"
provision_scripts = [
  "scripts/debian/qemu-ga.sh",
  "scripts/debian/cleanup.sh",
]
