# ADR-0046: Run NetBox as Podman containers on a node3 VM

- **Status:** Accepted
- **Date:** 2026-09-10
- **Related:** [ADR-0020](0020-tf-tree-axes-host-vs-cluster.md),
  [ADR-0024](0024-shared-proxmox-node-inventory-for-monitoring.md),
  [ADR-0025](0025-run-meshcentral-outside-managed-cluster.md),
  [`ansible/roles/netbox`](../../ansible/roles/netbox/README.md)

## Context

NetBox was installed from a GitHub release tarball on the `netbox1` LXC guest:
apt-provided PostgreSQL, Redis and nginx, a virtualenv built from
`requirements.txt`, and two hand-written systemd units. Every component version
was whatever the guest's Ubuntu release shipped, the build toolchain
(`build-essential`, `libxml2-dev`, `libpq-dev`, …) had to stay installed to
rebuild the virtualenv, and an upgrade meant re-running migrations and
`collectstatic` from Ansible.

Upstream publishes `netbox-community/netbox-docker`, whose image tags pin both
the NetBox release and the packaging release, and whose entrypoint already does
the migration, `collectstatic` and superuser work the role was reimplementing.
Adopting it fixes PostgreSQL and Valkey versions in the same declaration.

Authentik is the existing precedent for that shape in this fleet: rootful
Podman with Quadlet units, on a VM. There is no precedent for Podman inside an
unprivileged LXC guest here, and validating one would be the largest single risk
in the change.

## Decision

**Deploy NetBox from the upstream netbox-docker images as five rootful Podman
Quadlet units, on a new Ubuntu VM on node3, cutting over through a second IP.**

- The VM is `tf/vm/node3/netbox` (2 cores, 4 GiB, 40 GiB), keeping both the
  `netbox1` inventory name and the old guest's `192.168.10.247`. node3 already hosts
  `authentik1`, the fleet's other Podman platform-service VM, and node2 — the
  most heavily loaded LXC host — gives back the old guest's memory.
- Units: `netbox`, `netbox-worker`, `netbox-postgres`, `netbox-redis` and
  `netbox-redis-cache`, on a dedicated `netbox` Podman network.
- Only `netbox` publishes a port. Granian serves `/static` itself, so nginx is
  dropped and Caddy proxies straight to port 8080.
- PostgreSQL and both Valkey instances run without passwords. They publish no
  ports and are reachable only from the `netbox` Podman network, so a password
  would protect nothing that the network boundary does not already close.
- Cutover is staged: build the VM on a temporary `192.168.10.249` alongside the
  running LXC guest, repoint the inventory, restore a dump, verify on that
  address, switch Caddy, then destroy `tf/lxc/node2/netbox` and move the VM onto
  the freed `192.168.10.247`.

## Alternatives considered

- **Containers on the existing LXC guest.** Rejected: rootful Podman inside an
  unprivileged LXC guest is unproven here, and proving it is a larger risk than
  the VM move it would avoid.
- **Keep the tarball install.** Rejected: it pins nothing but the NetBox version
  and keeps a compiler toolchain on a service guest.
- **Move NetBox into the prd cluster.** Rejected: it would put the IPAM record
  of the fleet inside the fleet, and every other platform service in this tier
  is deliberately outside it.
- **node4 or node2 for the VM.** node4 has the most headroom but ADR-0021
  reserves it as a small always-on controller host. node2 would leave the
  fleet's busiest LXC host carrying a 4 GiB VM as well.

## Consequences

- Component upgrades become image tag changes; `netbox_version` and
  `netbox_docker_version` are the two halves of the tag and are tracked
  separately by Renovate.
- `netbox1` moves from the `lxc` inventory group to `node_exporter_vm`, and its
  monitoring target IP changes. Its Vector agent is dropped, matching the other
  VM guests — NetBox journald logs stop reaching Loki until VM-side log
  collection is addressed as its own piece of work.
- The PostgreSQL move is a `pg_dump`/`pg_restore`, not an in-place upgrade, so
  the old guest must stay recoverable until the new one is verified.
- `ops-version_audit.yaml` reads the NetBox version from a symlink target and
  needs a container-aware replacement.
