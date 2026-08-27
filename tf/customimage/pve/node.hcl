locals {
  node_name = "pve"
  image_keys = [
    "ubuntu-24.04-base",
    "ubuntu-24.04-tool",
    "ubuntu-24.04-desktop",
    "rocky-9-base",
    "rocky-9-desktop",
    "rocky-10-base",
    "debian-13-base",
    "freebsd-151-cloudinit",
  ]
}
