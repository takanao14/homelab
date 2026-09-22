# ADR-0053: Resolve from public DNS on rpi4

- **Status:** Accepted
- **Date:** 2026-09-22
- **Related:** [ADR-0002](0002-dhcp-outside-proxmox-cluster-nodes.md),
  [ADR-0025](0025-run-meshcentral-outside-managed-cluster.md),
  [ADR-0030](0030-in-house-full-service-resolver.md),
  [ADR-0051](0051-forward-recursive-cache-misses-over-dot.md),
  [ADR-0052](0052-static-addressing-for-rpi3.md),
  [`ansible/roles/netplan/README.md`](../../ansible/roles/netplan/README.md)

## Context

ADR-0030 made resolver1 and resolver2 the in-house full-service resolvers,
reached by clients through the dnsdist front ends dist1 and dist2. All four are
LXC guests on the Proxmox cluster.

rpi4 exists to stay outside that failure domain. It serves AMT addressing
(ADR-0002) and runs MeshCentral (ADR-0025), which are the tools used to recover
a cluster node that is powered off, hung, or has a broken OS. Pointing it at
dist1 and dist2 would make name resolution on the recovery host depend on guests
of the cluster it recovers.

Its previous resolvers were bgw1 and 8.8.8.8. ADR-0051 recorded bgw1's DNS
timing out during an incident and mishandling DNSSEC-bogus answers, so it is not
a dependency worth keeping on this host.

## Decision

Resolve on rpi4 through two public resolvers, 9.9.9.9 and 8.8.8.8, declared in
its `host_vars` and applied by the `netplan` role. Do not point rpi4 at the
in-house resolvers.

## Rationale

Two operators, because a single provider failing would take the recovery path
with it. Quad9 validates DNSSEC upstream, which matters here because rpi4 runs
no local validator — unlike a client of ADR-0030's resolvers, it cannot rely on
validation happening on the way.

## Consequences

- rpi4's queries leave the network, so they are absent from in-house DNS
  telemetry and outside ADR-0030's local validation and query-locality
  properties. That is accepted for one host whose job is to work when the
  cluster does not.
- The asymmetry with rpi3, which keeps dist1 and dist2 (ADR-0052), is
  deliberate. rpi3 is not on the recovery path.
