generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents  = file("${get_parent_terragrunt_dir()}/provider.tf")
}

remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    bucket = "homelab-tfstate"
    key    = "${path_relative_to_include()}/terraform.tfstate"
    region = "auto"

    # tf/.envrc injects the account endpoint and S3 credentials.
    endpoints = {
      s3 = get_env("R2_S3_ENDPOINT")
    }

    # Disable AWS-only checks for R2.
    use_path_style              = true
    skip_credentials_validation = true
    skip_metadata_api_check     = true
    skip_region_validation      = true
    skip_requesting_account_id  = true
    skip_s3_checksum            = true

    # Native conditional-write locking; no DynamoDB.
    use_lockfile = true
  }
}

inputs = {
  password       = get_env("TF_VM_PASSWORD")
  ssh_public_key = get_env("TF_VM_SSH_PUBLIC_KEY")
}
