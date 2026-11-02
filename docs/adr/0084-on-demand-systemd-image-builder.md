# ADR-0084: Run image builds on an on-demand systemd builder VM

- **Status:** Accepted
- **Category:** Provisioning
- **Date:** 2026-11-01
- **Related:** [ADR-0006](0006-custom-image-pipeline-monorepo-and-seaweedfs-s3.md),
  [ADR-0031](0031-browser-editor-on-relocated-toolbox-vm.md),
  [ADR-0032](0032-reboot-and-planned-shutdown-orchestration.md),
  [ADR-0077](0077-select-guest-os-by-role.md),
  [`packer/README.md`](../../packer/README.md)

## Context

Interactive work needs responsive, persistent sessions. Packer builds consume
CPU, memory and scratch storage and need KVM. Sharing their execution host
couples build maintenance and resource pressure to the remote work environment.

## Decision

Run builds on `imagebuilder1`, a reusable Ubuntu LTS base-image VM on `pve`,
with a separate scratch disk. Keep it off between jobs. The work VM or an
administrative terminal starts it through a VM-scoped Proxmox API token and
submits a pinned repository revision through a restricted SSH dispatcher.

Use systemd from the first build: the worker owns execution independently of
the terminal, while privileged dispatch and finalization own job admission,
credential cleanup and graceful shutdown. Serialize job admission and shutdown
under one lock. Do not queue or automatically resume interrupted jobs.

Terraform owns VM configuration and the initial stopped state. Only this
stack generates a Terraform override that ignores subsequent `started`
changes. The shared resource address and existing VM power management remain
unchanged. Keep `on_boot = false`; fleet recovery must not wait for this VM.

Keep publication separate from building and disabled unless requested. Preserve
the existing S3 image/checksum contract and Proxmox registration boundary.
Fixed-name pairs cannot be replaced atomically: exclude consumer operations
during publication, retain the previous pair, verify the uploaded pair and
restore on failure. An unresolved publication blocks later jobs until repaired.

Store job credentials only in tmpfs; do not copy the AGE key or administrative
environment to the builder. Treat local caches and worker disks as disposable.
Retain a verified base-image recovery copy outside routine replacement and
cleanup. If it is unavailable, a manually verified upstream image is permitted
only to bootstrap the builder; service VMs retain ADR-0077's base-image policy.

## Alternatives considered

- **tmux as an intermediate execution path** duplicates lifecycle handling;
  systemd is also used for initial manual validation.
- **KubeVirt** adds a virtualization layer above the existing VM-based workers
  and another cluster dependency for a single builder.
- **A new VM for every job** removes caches and requires provisioning on every
  invocation; per-job work directories give isolation without that lifecycle.
- **Terraform for routine power operations** couples builds to state access
  and conflicts with the separation between provisioning and job execution.

## Consequences

Builder failure does not terminate remote work sessions. Builds remain possible
without Kubernetes, although publication still depends on SeaweedFS. Capacity,
nested KVM, guest smoke tests and publication recovery require deployment-time
validation.
