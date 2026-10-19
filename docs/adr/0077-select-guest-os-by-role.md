# ADR-0077: Select the guest OS by role

- **Status:** Accepted
- **Category:** Provisioning
- **Date:** 2026-10-19
- **Related:** [ADR-0037](0037-bake-shared-toolchain-into-server-images.md),
  [ADR-0038](0038-image-role-suffixes-base-tool-desktop.md),
  [ADR-0070](0070-no-staging-sandbox-validates-the-cluster-platform.md),
  [ADR-0075](0075-debian-for-dns-lxc-guests.md)

## Context

Guests run Ubuntu 24.04, Ubuntu 26.04 and Debian 13, chosen per host by
creation date rather than by rule. ADR-0075 measured smaller package sets, disk
usage and upgrade memory peaks on Debian for DNS guests, but left every other
guest undecided.

Terraform declarations do not record the running OS. `proxmox-vm` ignores
changes to `disk[0].file_id`, so an in-place release upgrade leaves the
declared image stale, and LXC templates are likewise only creation inputs.

## Decision

Select the distribution by guest role:

| Role | Distribution |
|------|--------------|
| Infrastructure services: LXC guests and single-purpose service VMs | Debian stable |
| Kubernetes nodes, including GPU nodes | Ubuntu LTS |
| Toolchain, desktop, CI runner and agent-evaluation VMs | Ubuntu LTS |
| Compatibility test VMs | Exempt |

Proxmox hosts follow Proxmox VE and are outside this policy.

New guests use the current release of their distribution: currently Debian 13
and Ubuntu 26.04. Ubuntu interim releases are not used. The previous release is
allowed only on existing guests and must be replaced one year before its
standard support ends. The sandbox cluster adopts a new Ubuntu LTS before prd.

Existing guests change distribution when they are next rebuilt; no rebuild is
scheduled only to apply this policy. Prefer recreating a guest over an in-place
release upgrade. When an in-place upgrade is unavoidable, update the guest's
declared image or template in the same change. Service VMs use the Packer
`base` image, not upstream cloud images.

## Consequences

- Debian and Ubuntu remain permanently supported in the Packer images, LXC
  templates and shared Ansible roles. Rocky and FreeBSD remain test-only.
- Infrastructure guests share the Proxmox hosts' distribution family. Footprint
  reductions beyond DNS are expected from ADR-0075 but not yet measured.
- GPU nodes stay on the distribution that ROCm supports directly, and toolchain
  VMs avoid upstream APT suites that lag Debian releases, as ADR-0037 found for
  HashiCorp.
- Release selection for LXC guests should come from one shared catalog rather
  than per-node defaults.
- `node_os_info` is the source of truth for the running release; drift between it
  and the declarations or this policy must be detectable.

## Alternatives considered

- **Ubuntu everywhere** — one distribution family, but discards the ADR-0075
  footprint gains and diverges from the hosts.
- **Debian everywhere** — weakens GPU and toolchain support for limited gain on
  hosts whose workloads mostly run in containers.
- **Keep per-host choice** — no migration cost, but the release mix keeps
  growing and upgrade deadlines stay untracked.
