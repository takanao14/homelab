locals {
  image_definitions = {
    "ubuntu-2404" = {
      url                 = "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"
      file_name           = "ubuntu-24.04.img"
      content_type        = "iso"
      overwrite_unmanaged = true
    }
    "ubuntu-2604" = {
      url                 = "https://cloud-images.ubuntu.com/resolute/current/resolute-server-cloudimg-amd64.img"
      file_name           = "ubuntu-26.04.img"
      content_type        = "iso"
      overwrite_unmanaged = true
    }
    "rocky-9" = {
      url                 = "https://ftp.iij.ad.jp/pub/linux/rocky/9/images/x86_64/Rocky-9-GenericCloud-Base.latest.x86_64.qcow2"
      file_name           = "rocky-9.img"
      content_type        = "iso"
      overwrite_unmanaged = true
    }
    "rocky-10" = {
      url                 = "https://ftp.iij.ad.jp/pub/linux/rocky/10/images/x86_64/Rocky-10-GenericCloud-Base.latest.x86_64.qcow2"
      file_name           = "rocky-10.img"
      content_type        = "iso"
      overwrite_unmanaged = true
    }
    "debian-13" = {
      url                 = "https://cloud.debian.org/images/cloud/trixie/latest/debian-13-genericcloud-amd64.qcow2"
      file_name           = "debian-13.img"
      content_type        = "iso"
      overwrite_unmanaged = true
    }
  }
}
