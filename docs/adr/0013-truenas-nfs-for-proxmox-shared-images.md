# ADR-0013: TrueNAS NFS for Proxmox shared image storage

- **Status:** Accepted
- **Date:** 2026-07-06
- **Related:** [ADR-0006](0006-custom-image-pipeline-monorepo-and-seaweedfs-s3.md),
  [`tf/README.md`](../../tf/README.md).

## Context

Custom Proxmox images are built with Packer, published to the SeaweedFS
`cloud-images` S3 bucket, then downloaded by each Proxmox node with
`proxmox_download_file`. This decouples image build from VM provisioning and gives
Terraform a checksum-pinned URL.

However, every Proxmox node currently keeps its own local copy under `local:iso`.
Large images make registration slow and put serving pressure on the single
SeaweedFS LXC. We want a shared Proxmox image datastore so nodes do not each need
their own copy.

## Decision

Create a dedicated NFS export on TrueNAS for Proxmox shared image storage.

The initial Proxmox storage should be limited to non-running-disk content:

- `iso`
- `snippets`
- `vztmpl`

Do not use this NFS storage for VM live disks (`images`) at first. VM disks stay
on the existing node-local datastores (`local-zfs`, `local-lvm`, `data-nvme`) until
we explicitly evaluate performance, failure behavior, backup, and snapshot needs
for NFS-backed VM disks.

SeaweedFS remains the image distribution source. TrueNAS NFS becomes the shared
Proxmox registration target.

## Requirements

- Host the export on TrueNAS, outside the Proxmox node failure domain.
- Restrict NFS access to the Proxmox nodes, preferably by explicit node IPs.
- Prefer NFSv4.1 for the Proxmox storage definition.
- Allow Proxmox root on the trusted node IPs to create and replace image files.
- Keep the export dedicated to Proxmox image artifacts; do not mix application
  data or general NAS shares into the same dataset.
- Size the dataset for all custom images plus stock cloud images, with headroom
  for rebuild overlap.
- Monitor TrueNAS capacity and NFS health before making this storage a dependency
  of more workflows.

## Alternatives Considered

- **Continue per-node downloads only**: simplest and already works, but duplicates
  data and repeatedly stresses SeaweedFS during image registration.
- **Expose SeaweedFS directly as a Proxmox filesystem**: rejected. S3/FUSE-style
  mounts are not a good substrate for Proxmox storage semantics.
- **Run NFS from a Proxmox node or the SeaweedFS LXC**: easier to create, but it
  puts the shared storage inside the Proxmox failure domain and competes with
  SeaweedFS for I/O and memory.
- **Use NFS for VM live disks immediately**: deferred. This has a larger blast
  radius and needs a separate performance and durability review.

## Consequences

- Proxmox image registration can target one shared datastore instead of local
  `local:iso` copies on every node.
- The SeaweedFS LXC remains smaller in operational scope: it serves image objects
  to TrueNAS/Proxmox registration flows, not repeated long-lived per-node copies.
- TrueNAS becomes part of the image registration path. If TrueNAS is down, new VM
  provisioning from shared images is impacted, but already-running VMs on local
  disks are not.
