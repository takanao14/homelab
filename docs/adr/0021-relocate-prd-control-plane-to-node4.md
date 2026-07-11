# ADR-0021: Relocate the prd control plane to node4 via k0s backup/restore

- **Status:** Accepted (executed 2026-07-11)
- **Date:** 2026-07-11
- **Related:** [ADR-0019](0019-merge-gpu-worker-into-prd-retire-dev-cluster.md),
  [ADR-0020](0020-tf-tree-axes-host-vs-cluster.md),
  [ADR-0012](0012-openbao-eso-cluster-rebuild-registration.md)

## Context

The prd k0s controller (cp1) shared node1 with worker1, so node1
maintenance took down both the control plane and the only standard
worker. node4 (EliteDesk) was added as a small always-on host whose
planned profile (secondary DNS, tailscale) matches a lightweight
controller. The k0s controller is `role: controller` only — not a
Kubernetes node, no Cilium/L2 involvement — so it can live on any routed
segment; workers only need TCP to it (API 6443, konnectivity 8132, join
9443).

Interim placement on node2/node3 was rejected: a controller should move
once, to its final home, because every move re-incurs the full fixed-IP
blast radius. Placement on pve was rejected to keep the control plane off
the GPU/experiment host.

## Decision

Move cp1 to node4 on a new SDN segment (vnets60, 192.168.60.11) using the
supported single-controller path: `k0s backup` on the old controller →
`k0s stop` → `k0sctl apply --restore-from` onto the new VM (cluster
identity, CA and kine DB carry over) → reconfigure workers to the new API
address. Cilium follows via `k8sServiceHost` derived from
`K0S_CONTROLLER_ADDRESSES` (helmfile re-apply). The tf stack is
`tf/k8s/prd/cp1` (node4-bound sibling stack per ADR-0020); the old
worker+cp stack was renamed `prd-cluster` → `workers-node1`.

A full rehearsal on sandbox (restore to a spare IP on the same host) was
a mandatory gate and is what surfaced findings 1–3 below before prd was
touched.

## Consequences

- prd spans three hosts/segments: cp1@node4 (net60), worker1@node1
  (net30, sole L2 announcer), gpuvm@pve (net20). All API traffic is now
  routed; a routing outage stalls scheduling/self-heal/ArgoCD/ESO while
  pods keep running (accepted homelab trade-off).
- Fixed-IP blast radius on address change: `k0s/env/prd.sh`, Prometheus
  kcm/scheduler endpoints + control-plane-metrics values, kubeconfigs
  (local + OpenBao copy), ansible inventory `prd-cp1`, OpenBao
  `openbao_k8s_host` group_var.

## Operational findings (validated live)

1. **k0sctl does not rewrite `/var/lib/k0s/kubelet.conf` on already-joined
   workers** (join tokens are first-join only). After `k0sctl apply`,
   sed the server IP in kubelet.conf on every worker and restart
   `k0sworker` within the ~5-minute NotReady window, or taint eviction
   begins. Client certs stay valid (CA unchanged).
2. **ESO outage is expected on any API IP change**: OpenBao's k8s auth
   `kubernetes_host` embeds the IP. Recover with the group_var update +
   `ops-openbao_register_cluster.yaml` (ADR-0012).
3. **Rolling back to an older kine DB requires rebooting all workers**:
   agent informers (Cilium, CSI, CoreDNS) hold resourceVersions newer
   than the restored DB and silently stop seeing new objects. Forward
   restores are immune (they continue from the backup revision).
4. **The old controller's masterlease survives in kine after restore**
   (no TTL expiry observed): the apiserver lease endpoint-reconciler then
   publishes both old and new IPs in the `kubernetes` Endpoints, so
   in-cluster API traffic (10.96.0.1) intermittently hits the dead IP.
   Gate: `kubectl -n default get endpoints kubernetes` must list only the
   new IP. Fix: stop k0s on the new controller, delete the
   `/registry/masterleases/<old-ip>` rows from `/var/lib/k0s/db/state.db`,
   start k0s.

Future HA note: `template_lib.sh` already supports multiple controllers
(etcd, odd count), but cross-segment etcd puts quorum behind the routing
path — revisit route availability first.
