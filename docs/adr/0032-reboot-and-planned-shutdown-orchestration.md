# ADR-0032: Orchestrate reboots and planned shutdowns from Ansible

- **Status:** Accepted
- **Date:** 2026-08-09
- **Related:** [ADR-0001](0001-service-oriented-ansible-playbook-organization.md),
  [ADR-0019](0019-merge-gpu-worker-into-prd-retire-dev-cluster.md),
  [ADR-0021](0021-relocate-prd-control-plane-to-node4.md),
  [ADR-0024](0024-shared-proxmox-node-inventory-for-monitoring.md),
  [`ansible/playbooks/ops-package_upgrade.yaml`](../../ansible/playbooks/ops-package_upgrade.yaml),
  [`ansible/playbooks/ops-shutdown.yaml`](../../ansible/playbooks/ops-shutdown.yaml),
  [`ansible/playbooks/ops-startup.yaml`](../../ansible/playbooks/ops-startup.yaml)

## Context

Two events take the fleet down, and until now neither was orchestrated.

**Rolling reboots.** `ops-package_upgrade.yaml` targeted `all:!proxmox:!dns`
with no `serial`, so every k0s node in both clusters upgraded and rebooted at
once. Nothing was cordoned or drained: kubelet and containerd were killed with
the host, and workloads got no SIGTERM. The two clusters fail differently under
that. sandbox runs Longhorn with `defaultReplicaCount: 3` across exactly three
workers, so every volume has a replica on every worker and a simultaneous
reboot takes all of them down together — past degraded, into faulted, relying
on auto-salvage to come back. prd runs OpenEBS LocalPV hostpath, where PVs are
node-local directories with node affinity and there are no replicas at all.

**Planned power outage.** No procedure existed. The dependency order, which
guests do not restart by themselves, and how workloads come back were all
undocumented.

Several facts shaped the design and are easy to get wrong later:

- Ansible runs from a workstation outside the fleet, so it survives every phase
  including powering off the hypervisors.
- The inventory addresses hosts by IP and sets `host_key_checking = False`, so
  Ansible itself does not depend on DNS. DNS ordering is an application
  concern, not an execution one.
- The Proxmox hosts are independent installations, not a cluster
  (`group_vars/proxmox.yaml`). There is no quorum to lose and no HA fencing to
  provoke.
- pve and node5 have no AMT (ADR-0024's inventory carries `amtIp` only for
  node1–node4), so they cannot be woken remotely.
- Inventory host names did not match the node names `kubectl get nodes`
  reports (`prd-worker1` vs `k0s-worker1`).

## Decision

**Treat the two events as different strategies over a shared foundation.**
Draining and scaling to zero are mutually exclusive, and conflating them is the
central mistake this ADR exists to prevent:

|                | Rolling reboot        | Planned shutdown |
| -------------- | --------------------- | ---------------- |
| Strategy       | cordon + drain        | scale to zero    |
| Why not the other | scaling discards availability that a rolling operation exists to keep | draining has nowhere to evict to, so it only makes Pending pods and burns the eviction timeout |
| Wait condition | node Ready, storage recovered | workloads stopped, volumes detached |
| Argo CD        | untouched             | application controller scaled to 0 |

Shared below that: node identity, k0s service names, and a storage-readiness
wait selected by `k8s_storage_providers` — controller readiness for OpenEBS
and NFS plus an endpoint mount probe for NFS. Adding a cluster adds group_vars,
not tasks.

**Align inventory host names with the k0s node names.** tf is canonical: the
`for_each` map key is the Proxmox VM name, the cloud-init hostname, and the
node name Kubernetes reports. Renaming there would change a resource address
and destroy/recreate the VM, so the inventory moved instead. `inventory_hostname`
is now usable directly as the node name, and no translation table exists.

**Express ordering in the inventory, not in playbooks.** Ansible executes a
group in the order its hosts are declared, and `order: reverse_inventory`
yields the exact reverse (both verified empirically). So `prd_k8s` and
`sandbox_k8s` gained `_controller` / `_worker` children, cross-cluster
`k8s_controller` / `k8s_worker` groups carry role-only variables, and
`guest_shutdown_order` / `proxmox_shutdown_order` encode the shutdown sequence
itself. Controllers are always processed after workers: draining a worker talks
to the API server the controller provides.

**Add a `power_only` group** for hosts Ansible sequences but does not
configure — `vpngw` today. Every `all:*` selector carries `:!power_only`, so
registering such a host cannot change what gets configured or rebooted.

**Suspend GitOps by scaling the Argo CD application controller to zero**, not
by patching each Application's `syncPolicy`. One resource, no per-Application
state to restore.

**Snapshot replica counts to a file outside the repo** before scaling down, and
restore from it. Argo CD only reverts a replica count its rendered manifests
declare, so a chart that leaves `replicas` unset would otherwise stay at zero.
The snapshot is taken before the application controller is stopped, or its own
count would be recorded as zero.

**Detect namespace drift dynamically, but only ever scale what was reviewed.**
Namespaces are classified statically into an ordered application list and an
infrastructure exclusion list, and the shutdown verifies the split covers every
namespace that carries a Deployment or StatefulSet. Scaling down whatever
happens to be running instead would make a forgotten infrastructure namespace a
casualty mid-shutdown; leaving the lists unverified would make a forgotten
application namespace a silent omission. The check fails the run by default and
can be waived with `-e k8s_shutdown_allow_unclassified=true`, since an outage is
a bad time to be editing group_vars.

**Compare failed systemd units before and after an upgrade** rather than
failing on any failed unit, so chronically broken services the upgrade neither
caused nor can fix do not abort the run.

**Recovery leans on firmware and `on_boot`, not on a playbook.** Every node has
BIOS AC power recovery set to power on, so mains returning starts the fleet;
guests follow via `on_boot` in tf. `ops-startup.yaml` waits for that to happen
in reverse shutdown order, starts the sandbox cluster (the only guests with
`on_boot = false`), and restores the workloads.

## Alternatives considered

- **Keep the Ansible names and map to node names with a variable.**
  *Rejected.* A translation table would have to be maintained in `host_vars`
  for every node and consulted by every task that speaks to the API.
- **Rename the VMs in tf to match the inventory.** *Rejected.* Changing a
  `for_each` key destroys and recreates the VM, and the node would have to be
  deleted and re-joined. Non-starter for prd.
- **Drain for the planned shutdown too, for symmetry.** *Rejected.* See above:
  with every node going down there is no eviction target.
- **Scale to zero for rolling upgrades too.** *Rejected.* It gives up the
  availability that draining one node at a time is meant to preserve.
- **Let Argo CD restore replicas after a shutdown.** *Rejected.* Silently
  leaves any workload whose chart omits `replicas` at zero.
- **Enumerate hosts per play to control ordering.** *Rejected.* Duplicates
  inventory knowledge in every playbook that needs a sequence, and drifts.
- **Drive OS upgrades through k0sctl.** *Not applicable.* k0sctl upgrades k0s,
  not the operating system. Its drain options were copied so both paths
  disrupt the cluster identically.
- **Add `startup { order, up_delay }` to the tf VM/container modules.**
  *Rejected.* Both `proxmox_virtual_environment_vm` and
  `proxmox_virtual_environment_container` support the block in the pinned
  provider, but Proxmox applies startup order **per node**, and these are six
  independent installations. The dependency that matters spans nodes — DNS
  lives on node2 and node3 while the k0s nodes live on node1, node4, node5 and
  pve — so `startup` cannot express it at all. Only `ops-startup.yaml` can, by
  waiting in dependency order across the fleet. Within a node the ordering buys
  little, since the services that start early enough to miss a dependency
  simply retry. node1, node4 and node5 host a single guest each, so the block
  would be inert there. That leaves one real benefit: `up_delay` staggering the
  eleven guests node2 starts at once, which is not worth changing both shared
  modules and node2's stacks for an event that happens a few times a year.

## Consequences

- Upgrade runs take considerably longer: DNS hosts and k0s nodes are processed
  one at a time, and each k0s node waits for storage to recover before the next
  begins. Phases are tagged so a partial run can be resumed.
- Every k0s node is drained on every upgrade run, even when no reboot turns out
  to be required — `/var/run/reboot-required` is only known after the upgrade,
  and maintainer scripts restart services regardless.
- Both playbooks were rehearsed end to end against pve, node1, node4 and node5,
  taking both clusters down entirely — 28 prd workloads and 16 sandbox ones
  scaled to zero and restored, Argo CD suspended and brought back first, k0s
  stopped, all eight nodes powered off, `truenas` stopped over ACPI, four
  hypervisors powered off and started again, and every workload back with no pod
  left outside Running. node2, node3, rpi3 and rpi4 stayed up, which kept DNS and
  therefore the operator's own name resolution alive throughout.
- What the rehearsal could not cover is the BIOS AC-power-recovery path: a
  graceful `shutdown -h now` leaves the machine in soft-off with mains still
  present, so the firmware setting never fires. Recovery used AMT for node1 and
  node4 and the physical button for pve and node5. Whether those two — the ones
  with no AMT — come back on their own after a real power cut is still unproven.
- Nothing here dry-runs usefully: the kubectl steps are `command` tasks, which
  check mode skips.
- During the original rehearsal, the former Longhorn detach wait proved that
  sandbox's Prometheus volume moved from `attached/healthy` to `detached` and
  back without a rebuild. ADR-0033 later removed Longhorn; the same
  scale-to-zero boundary now protects OpenEBS LocalPV and NFS workloads.
- Waiting on each scaled workload's `.status.replicas` rather than on the
  namespace emptying is load-bearing, not stylistic: node-exporter DaemonSet
  pods stay up until their node stops, so a namespace-level wait would never
  return.
- The replica snapshot is runtime state outside the repo. Losing it means
  restoring replica counts by hand.
- Recovery depends on the BIOS setting staying in place. If it is ever reset,
  pve and node5 need physical access — they have no AMT fallback.
- Guests that exist on a hypervisor but in neither tf nor the inventory —
  `truenas` on pve today — are stopped over ACPI in the hypervisor phase, listed
  per host in `shutdown_unmanaged_guests`. Reconciling tf against the inventory
  does not surface them; only comparing against what is actually running does,
  and the "guests are still running" assert is what makes their absence loud
  rather than silent.
- Hypervisor shutdown order matters only for the guests each node carries;
  there is no quorum consideration. The `ha-manager` check is kept as cheap
  insurance should a cluster ever be formed.
