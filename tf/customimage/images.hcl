locals {
  # Custom images built by Packer (see ../../packer). file_name is the basename
  # under packer/images/; the full path is assembled in each env's terragrunt.hcl.
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
    "debian-13-custom" = {
      file_name    = "debian-13-custom.img"
      content_type = "iso"
    }
  }
}
