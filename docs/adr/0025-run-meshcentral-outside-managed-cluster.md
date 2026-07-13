# ADR-0025: Run MeshCentral outside the managed Proxmox and Kubernetes fleet

- **Status:** Accepted
- **Date:** 2026-07-13
- **Related:** [ADR-0002](0002-dhcp-outside-proxmox-cluster-nodes.md),
  [ADR-0019](0019-merge-gpu-worker-into-prd-retire-dev-cluster.md),
  [`ansible/roles/meshcentral`](../../ansible/roles/meshcentral/README.md)

## Context

MeshCentral provides the out-of-band management path for the Proxmox hosts.
Running it in the prd k0s cluster makes that recovery path depend on the fleet
it is intended to recover: worker1 on node1, the controller on node4, Cilium,
Envoy Gateway, and cluster storage must all remain sufficiently healthy.

The existing `rpi4` host is outside both failure domains and already provides
AMT DHCP for the same recovery path (ADR-0002). It has 7.6 GiB RAM and 98 GiB
free local storage; `rpi3` has only 905 MiB RAM. The published MeshCentral
image includes a native `linux/arm64` manifest for both hosts' architecture.

## Decision

Run MeshCentral on `rpi4` as a resource-limited Podman container managed by
systemd and provisioned with a service-oriented Ansible playbook. Persist all
application state under `/opt/meshcentral` on local storage. Caddy provides the
public TLS endpoint after the fresh instance is configured and tested directly.
Use `meshcentral.home.butaco.net` as the canonical hostname because the service
is independent of the `prd` Kubernetes environment.
Do not migrate the prd Kubernetes PVCs: the earlier dev-to-prd move did not
preserve the MeshCentral configuration, so there is no useful server state to
carry forward. Re-register managed devices against the standalone instance.

Keep Kea and MeshCentral independently relocatable even though they share the
host. Limit MeshCentral to 1 GiB RAM and one CPU so that application failure or
load cannot starve AMT DHCP.

## Alternatives considered

- **Keep MeshCentral in prd k0s.** Rejected because it preserves the circular
  recovery dependency.
- **Run it on a Proxmox VM or LXC.** Rejected because the host remains part of
  the managed failure domain.
- **Run it on rpi3.** Rejected because its 905 MiB total RAM is below the
  existing 1 GiB application limit and would rely on swap under load.
- **Add a dedicated host.** Operationally clean, but unnecessary at the current
  scale given rpi4's measured headroom.

## Consequences

- Proxmox and k0s outages no longer remove the MeshCentral server.
- A full rpi4 outage removes both AMT DHCP and its management UI. Direct access
  to already leased AMT addresses remains a break-glass path; a future second
  recovery host can separate these services without changing their roles.
- Deployment moves from ArgoCD/Helm to Ansible/systemd. Image upgrades remain
  explicit version changes in the role defaults.
- Cutover is staged: deploy on port 8443, configure and validate the fresh
  instance, switch Caddy, re-register devices, then remove the Kubernetes
  application.
