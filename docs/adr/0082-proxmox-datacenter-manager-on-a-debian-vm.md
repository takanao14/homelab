# ADR-0082: Run Proxmox Datacenter Manager on a Debian service VM

- **Status:** Accepted
- **Category:** Configuration & operations
- **Date:** 2026-10-30
- **Related:** [ADR-0008](0008-caddy-https-upstream-for-self-signed-backends.md),
  [ADR-0024](0024-shared-proxmox-node-inventory-for-monitoring.md),
  [ADR-0032](0032-reboot-and-planned-shutdown-orchestration.md),
  [ADR-0077](0077-select-guest-os-by-role.md),
  [`ansible/roles/pdm`](../../ansible/roles/pdm/README.md)

## Context

The six Proxmox VE hosts are independent installations, not a cluster
(ADR-0032). Each has its own web UI, and moving a guest between hosts needs
`qm remote-migrate` with hand-built API tokens. Proxmox Datacenter Manager
(PDM, 1.1 stable) adds one view over independent remotes, cross-remote live
migration, and a pending-update overview, without the hosts depending on it.

PDM overlaps with existing tooling: metrics and alerts live in
node-exporter, Loki and Grafana (ADR-0024); updates and reboots are
orchestrated by `ops/package_upgrade.yaml` (ADR-0032); guest placement is
declared in Terraform, where a node change is a resource change.

## Decision

- Run PDM on `pdm1`, a Debian 13 service VM built from the Packer `base` image
  (ADR-0077), and install it from the `pdm-no-subscription` APT repository with
  `proxmox-datacenter-manager-container-meta`, which keeps the Debian kernel.
  Install it without Recommends: `proxmox-datacenter-manager` recommends the
  full meta package, `ifupdown2` and ZFS, which on the cloud image fail in
  `grub-pc` and add a second network stack beside cloud-init. The PDM features
  they enable manage the PDM host itself, which Terraform and Ansible own here.
  Packages that a later PDM release newly recommends still arrive through the
  fleet's `dist-upgrade`.
- Place it on node4: running guests stay at 8 of 15.4 GiB, while node1, node2,
  node3 and node5 are already at 88% or more of physical memory. pve has room,
  but its available memory fell from 44 to 11 GiB within a week and it is one
  of the two dependency-heavy hosts (ADR-0032).
- Size it to the documented recommended requirements (2 vCPU, 4 GiB, 40 GB).
  The documentation gives no per-remote sizing; the evaluation minimum is not
  for production use.
- Publish the UI as `pdm.home.butaco.net` through Caddy with an HTTPS
  upstream (ADR-0008). PDM reaches the remotes directly; it does not support a
  reverse proxy on that leg.
- Scope PDM to observation and to migrating guests that Terraform does not
  manage. Apply updates through `ops/package_upgrade.yaml`, never from PDM,
  and move Terraform-managed guests only by changing their declarations.
- Treat PDM state as rebuildable: remote registrations, their API tokens and
  realms are recreated after a rebuild, and cached metrics are disposable. No
  PDM backup is kept.
- Upgrade PDM in place with the rest of the fleet. Major Debian releases follow
  ADR-0077 and rebuild the VM.

## Alternatives considered

- **ISO installer** — the documented default, but an interactive install
  outside Terraform and cloud-init, and the unattended path needs a custom ISO
  pipeline. Still the right choice if PDM moves to dedicated hardware.
- **Dedicated hardware outside the fleet** — would keep PDM up when its host
  is down, but no spare machine exists and PDM is not on any recovery path.
- **Clustering the Proxmox hosts** — gives native migration but adds quorum
  and fencing to hosts that are shut down and rebooted independently.
- **Evaluation-minimum sizing** — fits the observed footprint but leaves
  problems open to an unsupported-sizing explanation for little saving.

## Consequences

- One more service VM to patch and monitor; it joins the shared VM roles,
  node-exporter targets and the shutdown order.
- Each Proxmox host gains a PDM API token whose secret lives only in PDM, so
  the tokens are outside SOPS and are reissued on rebuild.
- When node4 is down, PDM and the view of every other host are unavailable;
  each host's own UI remains the fallback.
