# ADR-0047: Collect service VM logs with a Vector journald agent

- **Status:** Accepted
- **Date:** 2026-09-11
- **Related:** [ADR-0003](0003-proxmox-host-log-collection-via-rsyslog-forwarding.md),
  [ADR-0046](0046-netbox-containers-on-a-node3-vm.md),
  [`ansible/roles/vector`](../../ansible/roles/vector/README.md)

## Context

ADR-0003 forwards hypervisor logs with a thin rsyslog layer and deliberately
keeps Vector off the nodes: processing stays off a hypervisor, and a host
incident should involve as few moving parts as possible. LXC guests took the
opposite route — a Vector agent reads journald directly and pushes to Loki,
preserving `_SYSTEMD_UNIT` and the rest of the journal metadata.

Service VMs had neither. `authentik1`, `openbao1`, `runner1`, `toolbox1`, and —
after ADR-0046 moved it off LXC — `netbox1` produced nothing in Loki, so an
Authentik or OpenBao incident had to be investigated by hand over SSH.

The LXC pipeline had also been copied verbatim into seven inventory files.
Only `include_units` differed, plus one `job` label, so extending that shape to
VMs would have duplicated 24 identical lines per host added.

## Decision

**Service VMs run the same Vector journald agent as LXC guests, and the shared
pipeline moves into the role's defaults.**

- The role's default `vector_config` is the journald-to-Loki pipeline labelled
  by `host` and `unit`. A host declares `vector_journald_units`;
  `vector_loki_extra_labels` carries the resolver's `job` label. Inventory
  outranks role defaults, so `log_collector` and `rpi4` keep their own
  pipelines unchanged.
- The `vector_vm` inventory group holds `authentik`, `code_server`,
  `forgejo_runner`, `netbox`, and `openbao`.
- `since_now: true` is mandatory. Without it a first run replays the entire
  journal; Loki rejects everything older than its chunk window and rate-limits
  the remainder, which cost roughly 460,000 entries on authentik1's first
  start. A checkpoint takes precedence once written, so restarts lose nothing —
  verified by stopping an agent and confirming the entries written during the
  gap arrived afterwards.
- journald keeps its stock syslog forwarding on VMs. The `vector_lxc` policy
  disables it only to avoid LXC AppArmor denials; that constraint does not
  exist on a VM, and a local `/var/log/syslog` stays useful when the host
  cannot reach Loki.
- `host` and `unit` are the only index labels. Authenticated users, request
  paths, and lease identifiers stay in the payload where LogQL still filters
  them.

## Alternatives considered

- **rsyslog forwarding to log1**, as ADR-0003 does for hypervisors. *Rejected:*
  it loses journal metadata, and its motivation — keeping a hypervisor minimal
  during a host incident — does not apply to a service VM.
- **Alloy on the VMs.** ADR-0003 named it the fallback, and its RPM would also
  cover Rocky guests. *Rejected for now:* it adds a second agent to maintain
  alongside Vector for no gain on Ubuntu VMs. It remains the answer if
  non-Debian guests ever need collection.

## Consequences

- Adding a VM costs one inventory group entry and its unit list. Roughly 200
  duplicated lines were removed, and the rendered configuration on all twelve
  pre-existing hosts is byte-identical.
- Ingest rises from about 425 MB/day. Loki holds 90 days in 20 GiB with under
  0.5 GiB used, so the five VMs need no capacity change.
- k0s node VMs, PoC guests, and Kubernetes Pod logs stay out of scope. Rocky
  guests cannot use this role at all, since it installs Vector through APT.
- `toolbox1` is deferred: its image-baked HashiCorp apt key predates a
  HashiCorp key rotation, so `apt-get update` fails before Vector installs.
  Its units are configured and it joins the group as soon as the key is fixed.
