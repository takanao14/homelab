locals {
  env    = read_terragrunt_config(find_in_parent_folders("env.hcl"))
  common = read_terragrunt_config(find_in_parent_folders("common.hcl"))

  user_config = {
    users = [{
      name                = local.env.locals.vm_defaults.username
      groups              = "wheel"
      shell               = "/bin/sh"
      ssh_authorized_keys = [for line in split("\n", file(get_env("TF_VM_SSH_PUBLIC_KEY"))) : trimspace(line) if trimspace(line) != "" && !startswith(trimspace(line), "#")]
    }]
  }
  ethernet_config = {
    gateway4 = local.common.locals.pve.net10.ipv4gw
    nameservers = {
      addresses = local.common.locals.dns_internal
      search    = [local.common.locals.dns_domain]
    }
  }
}
