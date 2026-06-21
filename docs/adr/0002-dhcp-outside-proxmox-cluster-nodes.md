# ADR-0002: Run DHCP outside the Proxmox / cluster nodes (on rpi4)

- **Status:** Accepted
- **Date:** 2026-06-21
- **Related:** [ADR-0001](0001-service-oriented-ansible-playbook-organization.md) (dhcp.yaml as a relocatable service), [`ansible/roles/kea/README.md`](../../ansible/roles/kea/README.md)

## Context

The Proxmox cluster nodes `node1`–`node3` have Intel AMT (vPro) out-of-band
management, reachable through MeshCentral. The AMT management interfaces live on a
dedicated subnet (`192.168.110.0/24` — `node{1,2,3}-amt` = `.11/.12/.13`) and
**obtain their IP addresses via DHCP**.

AMT exists to manage a node when it is otherwise unreachable: remote power control
and KVM while the node is powered off, hung, or has a broken OS. In exactly those
situations the node itself cannot be relied on to run any service.

## Decision

Run the DHCP server (`kea`) on **rpi4** — a small, always-on host that is *outside*
the Proxmox nodes and the k0s cluster — rather than on a Proxmox node or inside the
cluster.

## Rationale

If DHCP ran on a Proxmox node (or in-cluster), a node outage would take DHCP down,
and AMT IP-address resolution would fail — precisely when AMT is needed most. That
is a circular dependency: the recovery tool (AMT) would depend on the failure
domain it is meant to recover. rpi4 is independent of the cluster's power and
health, so DHCP — and therefore AMT addressing — stays available during node
outages.

## Consequences

- rpi4 becomes infrastructure that must stay up for out-of-band recovery to work;
  its availability is now part of the recovery path, not just a convenience.
- Consistent with ADR-0001, `dhcp.yaml` remains a relocatable service — but any
  relocation target must likewise sit **outside the Proxmox / cluster failure
  domain** (e.g. another always-on host, as in the EliteDesk expansion plan), not
  on a cluster node.
