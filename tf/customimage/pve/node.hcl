locals {
  node_name = "pve"
  image_keys = [
    "ubuntu-24.04-custom",
    "ubuntu-24.04-xrdp",
    "rocky-9-xrdp",
    "rocky-9-custom",
    "rocky-10-custom",
    "debian-13-custom",
    "freebsd-151-cloudinit",
  ]
}
