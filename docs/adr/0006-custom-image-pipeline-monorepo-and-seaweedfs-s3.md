# ADR-0006: Custom image pipeline — monorepo build + SeaweedFS S3 distribution

- **Status:** Accepted
- **Date:** 2026-06-21
- **Related:** [`packer/README.md`](../../packer/README.md),
  [`ansible/roles/seaweedfs/README.md`](../../ansible/roles/seaweedfs/README.md).
  Both implementation plans (`docs/plans/packer-monorepo-consolidation.md` and
  `docs/plans/seaweedfs-custom-image-flow.md`) have been removed now that the
  pipeline is executed and verified end to end on real hardware; see git history.

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
- **DNS for the download must resolve on the Proxmox nodes.** The download runs
  on the Proxmox node, not the apply host; the nodes could not resolve the
  internal `s3.home.butaco.net`. Resolved with a per-node `/etc/hosts` entry
  (`192.168.10.244 s3.home.butaco.net`, the Caddy fronting SeaweedFS) rather than
  changing the resolver — surgical, keeps TLS, avoids breaking ACME / split-horizon.
  Every node that downloads (dev pve, prd node1/node2/node3) needs the entry.
- **The SeaweedFS LXC must be sized for *serving*, not just storing.** In an
  (unprivileged) LXC the page cache counts against the memory cgroup, so RAM and
  file cache share one cap. Serving the multi-GB custom images (e.g. the xrdp
  desktop variants) drove the cgroup to its limit and the kernel OOM-killed
  `weed` mid-download. 2GB then 4GB both proved insufficient as the image set
  grew; the LXC now runs **8GB RAM + 4GB swap** (`tf/lxc/node3/seaweedfs`). This
  required adding an optional `swap` field to the `proxmox-container` module
  (defaults to 0, leaving other containers unchanged).
