locals {
  # Packer images in SeaweedFS; sidecar checksums detect same-URL rebuilds.
  base_url = "https://s3.home.butaco.net/cloud-images"

  # The role suffix says how much is baked in, and each role contains the one
  # before it: base (agent + timezone) < tool (+ shared CLI toolchain) <
  # desktop (+ XFCE/XRDP and the GUI applications). See packer/README.md.
  image_definitions = {
    "ubuntu-24.04-base" = {
      file_name    = "ubuntu-24.04-base.img"
      content_type = "iso"
    }
    "ubuntu-24.04-tool" = {
      file_name    = "ubuntu-24.04-tool.img"
      content_type = "iso"
    }
    "ubuntu-24.04-desktop" = {
      file_name    = "ubuntu-24.04-desktop.img"
      content_type = "iso"
    }
    "ubuntu-26.04-base" = {
      file_name    = "ubuntu-26.04-base.img"
      content_type = "iso"
    }
    "ubuntu-26.04-tool" = {
      file_name    = "ubuntu-26.04-tool.img"
      content_type = "iso"
    }
    "ubuntu-26.04-desktop" = {
      file_name    = "ubuntu-26.04-desktop.img"
      content_type = "iso"
    }
    "rocky-9-base" = {
      file_name    = "rocky-9-base.img"
      content_type = "iso"
    }
    "rocky-9-tool" = {
      file_name    = "rocky-9-tool.img"
      content_type = "iso"
    }
    "rocky-9-desktop" = {
      file_name    = "rocky-9-desktop.img"
      content_type = "iso"
    }
    "rocky-10-base" = {
      file_name    = "rocky-10-base.img"
      content_type = "iso"
    }
    "rocky-10-tool" = {
      file_name    = "rocky-10-tool.img"
      content_type = "iso"
    }
    "debian-13-base" = {
      file_name    = "debian-13-base.img"
      content_type = "iso"
    }
    "freebsd-151-cloudinit" = {
      file_name    = "freebsd-15.1-cloudinit-ufs.img"
      content_type = "iso"
    }
  }
}
