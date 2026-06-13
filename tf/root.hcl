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

    # Cloudflare R2 endpoint. Account-specific, injected via env (tf/.envrc),
    # so the account id is never committed. Credentials (AWS_ACCESS_KEY_ID /
    # AWS_SECRET_ACCESS_KEY) are read from the environment by the s3 backend.
    endpoints = {
      s3 = get_env("R2_S3_ENDPOINT")
    }

    # R2 is S3-compatible but not AWS: disable AWS-only validations/behaviours.
    use_path_style              = true
    skip_credentials_validation = true
    skip_metadata_api_check     = true
    skip_region_validation      = true
    skip_requesting_account_id  = true
    skip_s3_checksum            = true

    # Native state locking via conditional writes (If-None-Match). No DynamoDB.
    use_lockfile = true
  }
}

inputs = {
  password       = get_env("TF_VM_PASSWORD")
  ssh_public_key = get_env("TF_VM_SSH_PUBLIC_KEY")
}
