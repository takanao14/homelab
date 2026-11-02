locals {
  node_name = "pve"
  image_keys = [
    "ubuntu-24.04-base",
    "ubuntu-26.04-base",
    "ubuntu-26.04-tool",
    "ubuntu-26.04-desktop",
    "rocky-9-base",
    "rocky-10-base",
    "rocky-10-desktop",
    "debian-13-base",
    "freebsd-151-cloudinit",
  ]
}
