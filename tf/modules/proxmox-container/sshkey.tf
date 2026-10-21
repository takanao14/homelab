data "local_file" "ssh_public_key" {
  filename = var.ssh_public_key
}

locals {
  ssh_authorized_keys = [
    for line in split("\n", data.local_file.ssh_public_key.content) : trimspace(line)
    if trimspace(line) != "" && !startswith(trimspace(line), "#")
  ]
}
