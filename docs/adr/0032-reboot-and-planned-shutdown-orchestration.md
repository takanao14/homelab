# ADR-0032: Orchestrate reboots and planned shutdowns from Ansible

- **Status:** Accepted
- **Date:** 2026-08-09
- **Related:** [ADR-0001](0001-service-oriented-ansible-playbook-organization.md),
  [ADR-0019](0019-merge-gpu-worker-into-prd-retire-dev-cluster.md),
  [ADR-0021](0021-relocate-prd-control-plane-to-node4.md),
  [ADR-0024](0024-shared-proxmox-node-inventory-for-monitoring.md),
  [`ansible/playbooks/ops-package_upgrade.yaml`](../../ansible/playbooks/ops-package_upgrade.yaml),
  [`ansible/playbooks/ops-shutdown.yaml`](../../ansible/playbooks/ops-shutdown.yaml),
  [`ansible/playbooks/ops-startup.yaml`](../../ansible/playbooks/ops-startup.yaml),
  [`k0s/template_lib.sh`](../../k0s/template_lib.sh)

## Context

Two uncoordinated events took the fleet down:

**Rolling reboots.** `ops-package_upgrade.yaml` rebooted every k0s node at once,
without cordon or drain. Workloads received no SIGTERM. sandbox's former
three-replica Longhorn volumes lost all three workers; prd's OpenEBS LocalPV has
no replicas and is node-affine.

**Planned power outage.** No procedure existed. The dependency order, which
guests do not restart by themselves, and how workloads come back were all
undocumented.

Design constraints:

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

**Use separate strategies over a shared foundation:**

|                | Rolling reboot        | Planned shutdown |
| -------------- | --------------------- | ---------------- |
| Strategy       | cordon + drain        | scale to zero    |
| Why not the other | scaling discards availability that a rolling operation exists to keep | draining has nowhere to evict to, so it only makes Pending pods and burns the eviction timeout |
| Wait condition | node Ready, storage recovered | workloads stopped, volumes detached |
| Argo CD        | untouched             | application controller scaled to 0 |

Both share node identity, k0s service names, and provider-specific storage
readiness. New clusters add group_vars, not tasks.

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

**Full-fleet recovery leans on firmware and `on_boot`; cluster-only recovery
explicitly starts its guests.** Every node has BIOS AC power recovery set to
power on, so mains returning starts the fleet and most guests follow through
`on_boot` in tf. `ops-startup.yaml` still resolves and starts every cluster VM
by name, making that path idempotent after a full outage and functional after a
single cluster was powered off without rebooting its hypervisors. Environment
tags (`prd` and `sandbox`) select the corresponding guest-start, readiness and
workload-restore path. Full recovery then waits for the remaining guests in
reverse shutdown order before restoring the workloads.

**Scope this to running clusters.** `k0s/create_cluster.sh <env> reset` still
reboots after `k0sctl reset` to clear Cilium, nftables and mount residue before
bootstrap. At that point no cluster remains to drain or restore, so the reboot
belongs to the k0s lifecycle. Reboots of serving clusters belong in Ansible.

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
- **Add Proxmox `startup { order, up_delay }`.** *Rejected.* Ordering is per
  hypervisor, but DNS and k0s dependencies span six independent hosts. Ansible
  can wait across the fleet; per-host staggering alone does not justify shared
  module changes.

## Consequences

- Upgrade runs take considerably longer: DNS hosts and k0s nodes are processed
  one at a time, and each k0s node waits for storage to recover before the next
  begins. Phases are tagged so a partial run can be resumed.
- Every k0s node is drained on every upgrade run, even when no reboot turns out
  to be required — `/var/run/reboot-required` is only known after the upgrade,
  and maintainer scripts restart services regardless.
- End-to-end rehearsal restored 28 prd and 16 sandbox workloads, eight k0s nodes,
  `truenas`, and four hypervisors. node2, node3 and both RPis stayed up for DNS.
- What the rehearsal could not cover is the BIOS AC-power-recovery path: a
  graceful `shutdown -h now` leaves the machine in soft-off with mains still
  present, so the firmware setting never fires. Recovery used AMT for node1 and
  node4 and the physical button for pve and node5. Whether those two — the ones
  with no AMT — come back on their own after a real power cut is still unproven.
- Nothing here dry-runs usefully: the kubectl steps are `command` tasks, which
  check mode skips.
- Two reboot implementations now exist: `ansible.builtin.reboot` for running
  clusters and `reboot_nodes` in `k0s/template_lib.sh` for the post-reset case.
  The duplication is accepted because they share no wait condition — one waits
  for a node to be Ready and its storage recovered, the other only for the host
  to report a new boot id. A reboot added anywhere else belongs in Ansible.
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
