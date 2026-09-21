# ADR-0052: Configure rpi3 with static addressing instead of a DHCP lease

- **Status:** Accepted
- **Date:** 2026-09-21
- **Related:** [ADR-0002](0002-dhcp-outside-proxmox-cluster-nodes.md),
  [ADR-0025](0025-run-meshcentral-outside-managed-cluster.md),
  [ADR-0035](0035-observe-ix2106-dhcp-leases-from-rpi4.md),
  [`ansible/roles/networkmanager/README.md`](../../ansible/roles/networkmanager/README.md)

## Context

rpi3 took its address from the IX2106 as a DHCP client, renewing 192.168.10.240
every two hours. Twice the renewal failed and NetworkManager 1.52.1 logged
`dhcp4 (eth0): state changed no lease`, removed the address, and then produced
no further log line at all — no retry, not even its own 45-second timeout. The
host kept running normally, with no kernel, link, storage, or power event, but
had no route off the box.

- 2026-08-29 18:03 to 2026-08-31 21:38 — about 51 hours.
- 2026-09-19 23:37 to 2026-09-21 19:16 — about 44 hours.

Both outages ended only with a power cycle. rpi3 hosts MeshCentral (ADR-0025),
so it is part of the out-of-band recovery path and cannot depend on a service
whose failure strands it. Its address also sits outside the IX2106 dynamic pool
`192.168.10.30-127` (ADR-0035) and never changes, so the lease bought nothing.
rpi4 already carries a static netplan address.

## Decision

Pin rpi3's eth0 profile to `ipv4.method manual` through the `networkmanager`
role, keeping the address, gateway, and resolvers it already used. Leave the
IX2106 static binding in place as a reservation so the address stays excluded
from the dynamic pool.

The profile, not a netplan file, is the managed object: NetworkManager rewrites
`/etc/netplan` into its own `90-NM-<uuid>.yaml` whenever a connection changes,
which deletes an Ansible-managed file on the first apply.

## Rationale

The lease conferred no addressing flexibility and added a recurring dependency
on both the IX2106 and a DHCP client implementation that fails closed and
silently. Static configuration removes the renewal entirely: the interface comes
up addressed even when DHCP, the router, or NetworkManager's client is
unhealthy. Fixing the symptom instead — a watchdog that restarts
NetworkManager — would keep the dependency and add a moving part to the recovery
path.

## Consequences

- Address changes for rpi3 now require an Ansible run, not a lease edit on the
  IX2106. The binding there must not be reassigned to another device.
- rpi3 no longer appears in the IX2106 lease observations (ADR-0035); inventory
  and DNS remain the source of truth for it.
- Hosts that legitimately want a lease are unaffected; the `networkmanager` group
  scopes the role to the hosts whose addressing is code.
