iso_url      = "https://cloud.debian.org/images/cloud/trixie/latest/debian-13-genericcloud-amd64.qcow2"
iso_checksum = "file:https://cloud.debian.org/images/cloud/trixie/latest/SHA512SUMS"
ssh_username = "debian"
distro       = "debian"
provision_scripts = [
  "scripts/debian/qemu-ga.sh",
]
cleanup_script = "scripts/debian/cleanup.sh"
# The shared toolchain is Ubuntu/Rocky only: its HashiCorp step resolves an apt
# suite from VERSION_CODENAME, and releases.hashicorp.com has no trixie suite.
install_toolchain = false
