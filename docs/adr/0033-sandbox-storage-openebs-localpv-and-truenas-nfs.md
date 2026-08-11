# ADR-0033: Use OpenEBS LocalPV and TrueNAS NFS for sandbox storage

- **Status:** Accepted
- **Date:** 2026-08-11
- **Related:** [ADR-0009](0009-longhorn-ui-exposed-through-authenticated-gateway-route.md),
  [ADR-0013](0013-truenas-nfs-for-proxmox-shared-images.md),
  [ADR-0032](0032-reboot-and-planned-shutdown-orchestration.md),
  [`k0s/README.md`](../../k0s/README.md)

## Context

sandbox uses Longhorn with three replicas across exactly three worker VMs, but
all of those VMs and their data disks share one Proxmox host and its local ZFS
pool. Longhorn therefore tolerates a worker VM failure while adding rebuild,
detach, salvage, and rolling-maintenance coordination, but it does not remove
the physical host, pool, or power failure domain.

The cluster currently has one persistent workload: a disposable 5 GiB
Prometheus PVC. This is the lowest-cost point to change the storage model.
prd already uses OpenEBS LocalPV hostpath and remains outside this decision.

TrueNAS runs as an unmanaged VM on the same Proxmox host and already serves a
dedicated NFS export for Proxmox image artifacts under ADR-0013. Reusing that
dataset for Kubernetes application data would violate its isolation rule.

## Decision

Run both OpenEBS LocalPV and the Kubernetes SIG Storage `csi-driver-nfs` in
sandbox. `openebs-hostpath` is the only default StorageClass; workloads use the
non-default `nfs` class only when their PVC explicitly selects it. Deploying
multiple providers is expressed as a list in both k0s environment variables and
Ansible group variables, rather than as mutually exclusive branches.

Serve sandbox NFS volumes from a dedicated TrueNAS dataset and export, separate
from the Proxmox image export and from any future prd export. Restrict clients
to the sandbox node IPs and prefer NFSv4.1 with hard mounts. The CSI driver
creates one subdirectory per PVC. Use `Retain` so deleting a Kubernetes PV does
not silently remove its NFS directory; reclamation is an explicit storage
operation. Set each provisioned PVC directory to mode `0777`: sandbox workloads
do not share a stable UID/GID, and root squashing can prevent kubelet from
changing ownership. Isolation is provided by mounting only the selected PVC
subdirectory into a pod; every authorized node is already trusted to mount the
cluster export directly.

Keep Prometheus TSDB on `openebs-hostpath`. Prometheus explicitly does not
support NFS for local storage because filesystem semantics can cause
unrecoverable corruption. This was also confirmed by the Prometheus process
warning on the temporary NFS-backed sandbox PVC. Recreate the disposable PVC
on OpenEBS without copying its metrics. Use NFS only for workloads whose
storage engines support it, especially explicit RWX consumers.

Longhorn, its UI route, its OpenBao policy, and all provider-specific lifecycle
logic are removed after OpenEBS and NFS validation. The uninstall follows the
Longhorn 1.12 procedure only after no Longhorn PVC, PV, or volume remains.

Keep prd on OpenEBS LocalPV. Adding NFS to prd is a later decision based on
sandbox operating experience, performance of large model caches, monitoring
durability, and acceptance of the TrueNAS/Proxmox failure domain.

## Alternatives considered

- **Keep Longhorn.** Retains worker-VM redundancy, but keeps operational
  complexity without separating the physical failure domain. Rejected for
  sandbox.
- **Use NFS as the only provider.** Simplifies class selection, but removes a
  local storage option and makes every stateful workload depend on TrueNAS.
  Rejected.
- **Reuse the Proxmox image export.** Avoids one dataset and export, but mixes
  unrelated lifecycle, quota, snapshot, and access policies and contradicts
  ADR-0013. Rejected.
- **Use `nfs-subdir-external-provisioner`.** Functional but legacy relative to
  the maintained CSI driver. Rejected for a new deployment.
- **Use `democratic-csi`.** Provides deeper TrueNAS integration but adds more
  credentials and control-plane coupling than the current subdirectory model
  needs. Deferred until per-PVC ZFS datasets, quotas, or native snapshots are
  required.
- **Use soft NFS mounts.** Avoids indefinite waits when the server disappears,
  but can surface I/O errors and corrupt active writes. Rejected.

## Consequences

- NFS-backed pods can move between workers and can use RWX without Longhorn
  replica rebuild waits.
- Prometheus remains bound to the worker that owns its OpenEBS LocalPV. NFS is
  not a supported way to make its local TSDB movable; durable or highly
  available metrics require a Prometheus remote-storage architecture instead.
- TrueNAS and its Proxmox host become single failure points for NFS-backed
  workloads. Planned shutdown must stop those workloads and release mounts
  before stopping TrueNAS; startup must verify a real NFS mount before restoring
  them.
- The startup mount probe runs as a temporary Kubernetes Pod with an inline NFS
  volume. A host-side mount would require interactive sudo immediately after a
  reboot, making unattended recovery depend on a vanished sudo timestamp.
- The startup mount probe runs as a temporary Kubernetes Pod with an inline NFS
  volume. A host-side mount would require interactive sudo immediately after a
  reboot, making unattended recovery depend on a vanished sudo timestamp.
- Requested PVC capacity is not a per-PVC TrueNAS quota. Dataset capacity,
  snapshots, replication, alerts, and recovery tests are separate TrueNAS
  responsibilities.
- Longhorn is fully removed after the NFS export and CSI path are validated,
  OpenEBS becomes the default class, and Prometheus is migrated to OpenEBS.
  ADR-0009 is superseded by this ADR.
