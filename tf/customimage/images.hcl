locals {
  # Custom images built by Packer (see ../../packer) and published to the
  # SeaweedFS cloud-images bucket by packer/push.sh. Proxmox downloads them
  # directly from `base_url` via proxmox_download_file. file_name is both the
  # object key in the bucket and the name on the Proxmox datastore; the sidecar
  # `<file>.sha256` is used as checksum_url so a rebuilt image (same URL, new
  # content) is detected and re-downloaded.
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
  }
}
