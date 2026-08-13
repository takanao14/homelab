locals {
  # Packer images in SeaweedFS; sidecar checksums detect same-URL rebuilds.
  base_url = "https://s3.home.butaco.net/cloud-images"

  image_definitions = {
    "ubuntu-24.04-custom" = {
      file_name    = "ubuntu-24.04-custom.img"
      content_type = "iso"
    }
    "ubuntu-24.04-xrdp" = {
      file_name    = "ubuntu-24.04-xrdp.img"
      content_type = "iso"
    }
    "rocky-9-xrdp" = {
      file_name    = "rocky-9-xrdp.img"
      content_type = "iso"
    }
    "rocky-10-custom" = {
      file_name    = "rocky-10-custom.img"
      content_type = "iso"
    }
    "rocky-9-custom" = {
      file_name    = "rocky-9-custom.img"
      content_type = "iso"
    }
    "debian-13-custom" = {
      file_name    = "debian-13-custom.img"
      content_type = "iso"
    }
    "freebsd-151-cloudinit" = {
      file_name    = "freebsd-15.1-cloudinit-ufs.img"
      content_type = "iso"
    }
  }
}
