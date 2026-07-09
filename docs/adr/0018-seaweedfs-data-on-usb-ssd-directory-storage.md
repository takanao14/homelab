# ADR-0018: SeaweedFS data on a node3 USB SSD via Proxmox directory storage

- **Status:** Accepted
- **Date:** 2026-07-09
- **Related:** [ADR-0006](0006-custom-image-pipeline-monorepo-and-seaweedfs-s3.md),
  `tf/lxc/node3/seaweedfs/terragrunt.hcl`,
  `tf/modules/proxmox-container` (`mount_points`). The migration plan
  (`seaweedfs-usb-ssd-storage.md` in the private plans repo) has been removed
  now that the migration is complete.

## Context

SeaweedFS (unprivileged LXC `seaweedfs1` on node3) stored its data dir
`/var/lib/seaweedfs` on the container rootfs in the node3 `local-lvm` thin
pool. The multi-GB `cloud-images` bucket (ADR-0006) pressured the thin pool,
the rootfs left little headroom, and bucket churn shared I/O and wear with
the host's primary disk.

## Decision

Move the entire data dir onto a dedicated USB SSD, attached as a
**Proxmox directory storage** (`usb-ssd`, ext4, mounted by UUID with
`nofail`) with a **Terraform-managed `mount_point` volume** on the container
(module `mount_points` support, 200G at `/var/lib/seaweedfs`, `backup=false`).

Supporting choices:

- **Directory storage + managed volume, not a raw `pct` bind mount:** for an
  unprivileged LXC, a Proxmox-managed volume gets the uid/gid idmap shift
  applied automatically; a raw bind mount would need a manual
  `chown 100000:100000` on the host.
- **`is_mountpoint=yes` + fstab `nofail` as the failure mode:** if the USB
  disk is absent, the storage is inactive and the container fails to start
  cleanly, instead of the host writing into an empty `/mnt` and filling the
  rootfs.
- **Whole data dir, same path:** master/volume/filer state moves together, so
  the Ansible role, systemd unit, and S3 config are unchanged
  (`seaweedfs_data_dir` stays `/var/lib/seaweedfs`).
- **Accepted risk — USB stability:** a disconnect stops the volume server.
  Impact is bounded: the tfstate primary is Cloudflare R2 (SeaweedFS holds
  the DR copy), so a SeaweedFS outage does not block Terraform; only
  `cloud-images` distribution pauses.

## Alternatives considered

- **Raw bind mount (`pct mp0`).** *Rejected* — manual idmap chown on the
  host, invisible to Terraform.
- **Grow the rootfs on local-lvm.** *Rejected* — keeps the churn on the thin
  pool and the host disk; capacity was only one of the drivers.

## Consequences

- The container config in `tf/lxc/node3/seaweedfs/terragrunt.hcl` declares
  the mount; the host-side storage (`pvesm add dir usb-ssd …`) remains a
  manual, node3-local runbook step (no Proxmox host-storage automation exists
  in `ansible/`).
- The rootfs `disk0` cannot shrink in place; freed blocks were returned to
  the thin pool via `fstrim` only. A clean recreate with a small rootfs
  remains a future option if thin-pool pressure returns.
