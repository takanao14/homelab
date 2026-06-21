# ADR-0003: Proxmox host log collection via rsyslog forwarding to a central Vector

- **Status:** Accepted
- **Date:** 2026-06-21
- **Related:** [`docs/plans/proxmox-loki-log-collection.md`](../plans/proxmox-loki-log-collection.md) (implementation plan, Phase 6 pending), [`ansible/roles/rsyslog/README.md`](../../ansible/roles/rsyslog/README.md), [ADR-0004](0004-alertmanager-single-notification-hub.md)

## Context

Proxmox VE 9 logs its management services (`pvedaemon`, `pveproxy`, `pvestatd`,
…) to `systemd-journald`. We want those host logs in Loki, but the repository
already runs a central Vector/syslog pipeline (`log1`, `192.168.10.243`) and processing should
stay off the hypervisors.

A virtualization platform also imposes a strict priority order:

```
running guests > Proxmox mgmt/cluster services > local journal retention > remote log completeness
```

Remote logging must never endanger anything to its left.

## Decision

Forward Proxmox host logs with a **thin rsyslog layer** on each node:

```
journald -> rsyslog imjournal -> bounded disk-assisted queue -> RFC 5424/TCP -> log1 (Vector) -> Loki
```

rsyslog is *not* the local log store (journald remains authoritative); it is only
a forwarder. Safety constraints are mandatory: a fixed disk-queue limit, **drop
on full instead of backpressure**, no modification of Proxmox services/journald,
no historical-journal replay on first run, and one-node-at-a-time rollout.

## Alternatives considered

- **Grafana Alloy on every node** — reads journald directly and preserves full
  metadata (`_SYSTEMD_UNIT`, `_BOOT_ID`, …), but adds a package/repo/service/state
  file/upgrade cycle and direct Loki connectivity to every hypervisor. *Rejected*
  for now; remains the fallback **if preserving complete journal metadata becomes
  more important than minimizing hypervisor changes**.
- **Vector on every node** — same metadata benefit, but duplicates the central
  Vector and adds another general-purpose agent per hypervisor. *Rejected.*
- **systemd-journal-remote** — journald-native but still needs a conversion/
  collection layer before Loki. *Rejected.*

## Consequences

- Hypervisors stay minimal (Debian's stock rsyslog, no Loki access, fewer moving
  parts to diagnose during a host incident).
- The trade-off is **lost journal metadata**: only what survives the syslog
  forward reaches Loki. If that metadata is needed later, switch to Alloy.
- For LXC guests on a node, journald `ForwardToSyslog` / the guest rsyslog is
  disabled so Vector reads the journal directly — this also removed the node2
  kernel AppArmor denials.
