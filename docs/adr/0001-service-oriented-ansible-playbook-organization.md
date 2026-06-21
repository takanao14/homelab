# ADR-0001: Service-oriented Ansible playbook organization

- **Status:** Accepted
- **Date:** 2026-06-21
- **Related:** [`ansible/README.md`](../../ansible/README.md) (resulting convention). The original implementation plan (`docs/plans/ansible-playbook-organization.md`) has been removed now that it is executed; see git history.

## Context

Cross-cutting components (`vector`, `journald`, `timezone`, `node_exporter`, ...)
reached hosts through inconsistent mechanisms: some were embedded in per-system
playbooks, some had a bulk playbook, some both, some neither. There was no uniform
policy for how a role deployed across many systems should be invoked.

A concrete defect made the inconsistency costly: there was **no way to bulk-upgrade
`vector` across all hosts**. The old `vector.yaml` targeted only `log_collector`
(`log1`), and no inventory group spanned every Vector host — `vector_lxc` excluded
the rpi4 DHCP host.

## Decision

Organize playbooks on a **service-oriented axis**:

- A playbook is the deployable unit of a *service* (relocatable), **named by the
  service, not by the host**. A single host may therefore be the target of several
  service playbooks.
- The class of each playbook is encoded in a **filename prefix** so the kind is
  obvious at a glance:
  - **System** — no prefix (`netbox.yaml`, `dhcp.yaml`).
  - **Cross-cutting** — `common-` (`common-vector.yaml`, `common-chrony.yaml`):
    the bulk / version-bump entry point, targeting a dedicated group.
  - **Day-2 / operational** — `ops-` (`ops-package_upgrade.yaml`).
- **System playbooks stay self-contained:** the shared log-shipping stack
  (`vector` + `journald`) is pulled in via the `lxc_logging` meta-role, so a single
  `ansible-playbook playbooks/<system>.yaml` still provisions the full host.
- **Every cross-cutting role owns a dedicated group plus a `common-<role>.yaml`
  playbook** so it can be rolled out fleet-wide in one run. A new `vector` group
  (`vector_lxc` + `dhcp`) covers every Vector host, including rpi4.

## Alternatives considered

- **Option Y — `site.yaml`-centric full separation** (every role fully decoupled
  from system playbooks). *Rejected.* For a homelab (dozens of systems, mostly
  single-service changes) the loss of single-playbook self-containment outweighs
  the single-source-of-truth benefit.
- **`timezone` group as `all:!proxmox`.** *Rejected.* The broad pattern would
  newly apply the role to k0s control-plane nodes and toolboxes, expanding
  behaviour onto important infra. An explicit `timezone` group keeps the touched
  set identical to before.

## Consequences

- **rpi4 hosting both `dhcp.yaml` and `blackbox_exporter.yaml` is correct, not an
  anomaly.** Each is an independently relocatable service that currently happens to
  co-locate on rpi4 (DHCP must live outside the Proxmox / cluster failure domain —
  see [ADR-0002](0002-dhcp-outside-proxmox-cluster-nodes.md); blackbox is an
  external, out-of-cluster probe vantage point). This is what makes the EliteDesk
  service-relocation plan straightforward.
- Host-named playbooks are avoided. `rpi3.yaml` was dissolved into
  `common-timezone.yaml` + `common-rsyslog.yaml`. `proxmox.yaml` is the one
  accepted exception — genuine host/platform config for the hypervisor nodes.
- `vector` version bumps across the whole fleet are now possible via
  `common-vector.yaml`.
- `common-vector.yaml` single-run semantics changed from `log1`-only (old
  `vector.yaml`) to all Vector hosts.
