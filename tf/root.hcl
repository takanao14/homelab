generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents  = file("${get_parent_terragrunt_dir()}/provider.tf")
}

remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    path = "${path_relative_to_include()}/terraform.tfstate"
  }
}

inputs = {
  password       = get_env("TF_VM_PASSWORD")
  ssh_public_key = get_env("TF_VM_SSH_PUBLIC_KEY")
}
