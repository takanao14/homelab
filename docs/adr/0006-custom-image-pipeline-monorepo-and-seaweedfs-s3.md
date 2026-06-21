# ADR-0006: Custom image pipeline — monorepo build + SeaweedFS S3 distribution

- **Status:** Accepted
- **Date:** 2026-06-21
- **Related:** [`docs/plans/seaweedfs-custom-image-flow.md`](../plans/seaweedfs-custom-image-flow.md) (remaining tasks: real-machine plan unverified), [`packer/README.md`](../../packer/README.md). The original consolidation plan (`docs/plans/packer-monorepo-consolidation.md`) has been removed now that it is executed; see git history.

## Context

A custom Proxmox image went through one logical pipeline split across two repos:

```
build image (packer) -> register on Proxmox (terraform) -> VMs consume it
```

The standalone `homelab-packer-proxmox` repo duplicated this repo's Terragrunt
structure (`root.hcl`, env dirs, `.enc.env` secrets) and carried a near-duplicate
of the `proxmox-cloudimage` module. Both repos already shared the same AGE key and
tooling baseline, so consolidation needed no secret re-encryption.

## Decision

1. **Consolidate the build side into the monorepo** as `packer/` (a subtree),
   eliminating the duplicate Terragrunt stack.
2. **Distribute built images through the SeaweedFS `cloud-images` S3 bucket**,
   decoupling the Packer build from Proxmox registration:

   ```
   packer/build.sh -> images/<file>.img (+ .sha256)
     -> packer/push.sh (rclone) -> SeaweedFS cloud-images bucket
        -> tf/customimage/<env>: proxmox_download_file (url + sha256 via run_cmd)
   ```

   `tf/customimage` shares the **`proxmox-cloudimage`** module with `tf/cloudimage`
   (stock images); the module gained optional `checksum` / `checksum_algorithm`.

## Alternatives considered

- **Local-file upload module** (`proxmox_virtual_environment_file` via a new
  `proxmox-image-upload` module). This was the *original* plan, because the two
  TF resources differ (`proxmox_download_file` needs a URL; the upload resource
  needs a local file path). *Superseded and removed* — routing images through S3
  lets Proxmox pull directly with a checksum, which unifies the stock and custom
  paths on one module and decouples build from deploy. The upload module was never
  merged to production.

## Consequences

- One repo, one Terragrunt stack, one shared image module for stock + custom.
- Build and deploy are decoupled: rebuilding/pushing an image does not touch
  Terraform; Terraform just references the S3 URL + checksum.
- `push.sh` uses `no_check_bucket=true` (the imagebuilder S3 identity cannot
  create buckets); rclone is provisioned via dotfiles.
- `tf/customimage` forces `-parallelism=1` (parallel large downloads timed out).
