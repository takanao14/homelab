# k0s Cluster Management

Manage k0s cluster lifecycle with k0sctl and Helmfile.

## Prerequisites

| Tool | Purpose |
|------|---------|
| `k0sctl` | Cluster setup / reset |
| `helmfile` / `helm` | Helm deployments for CNI, storage, and device plugins |
| `kubectl` | Cluster readiness checks and helmfile hooks |
| `cilium` CLI | Wait for Cilium to become ready |
| `ssh` | Post-reset node reboots |

## Directory Structure

```
k0s/
├── create_cluster.sh              # Entry point: ./create_cluster.sh <env> <command>
├── remove-known-hosts.sh          # Remove stale SSH host keys for every node in an environment
├── template_lib.sh                # Shared library: k0sctl config generation and cluster management logic
├── helmfile.yaml.gotmpl           # Helm releases (Cilium, enabled storage providers, device plugin)
├── env/
│   ├── prd.sh                     # Prd non-secret variables (committed)
│   └── sandbox.sh                 # Sandbox non-secret variables (committed)
├── charts/
│   └── cilium-config/             # Local chart for Cilium L2 policy and IP pool
├── values/
│   ├── amd-device-plugin.yaml     # AMD GPU Device Plugin Helm values
│   ├── cilium.yaml.gotmpl         # Cilium Helm values
│   ├── cilium-config.yaml.gotmpl  # cilium-config Helm values (IP pool range)
│   ├── openebs.yaml.gotmpl        # OpenEBS Helm values and default-class selection
│   └── nfs.yaml.gotmpl            # NFS CSI StorageClass and Helm values
├── hook/
│   ├── ssdsetup.sh                # Format and mount SSD on worker node
│   └── mirror.sh                  # Configure containerd docker.io mirror
└── scripts/
    └── wait-cilium-crds.sh        # Helmfile presync hook: wait for Cilium CRDs
```

## Environment Variables

Cluster topology and non-secret settings live in `env/`. Shell variables may
override operator-specific behavior.

### Environment files (`env/prd.sh` / `env/sandbox.sh`)

| Variable | Description |
|----------|-------------|
| `K0S_CONTROLLER_ADDRESSES` | Comma-separated controller node IP addresses |
| `K0S_WORKER_ADDRESSES` | Comma-separated worker node IP addresses |
| `K0S_GPU_WORKER_ADDRESSES` | Comma-separated GPU worker IP addresses (optional; omit for no GPU workers) |
| `K0S_LB_POOL` | Cilium LoadBalancer IP pool range (`start,stop`) |
| `K0S_VERSION` | k0s version to install (optional; omits `version:` if unset) |
| `K0S_STORAGE_PROVIDERS` | Comma-separated storage providers to deploy: `openebs` and/or `nfs` |
| `K0S_DEFAULT_STORAGE_CLASS` | The single default class: `openebs-hostpath` or `nfs`; its provider must be enabled |
| `K0S_NFS_SERVER` | NFS server IP or name; required when `nfs` is enabled |
| `K0S_NFS_SHARE` | Absolute NFS export path; required when `nfs` is enabled |

### Optional shell variables

| Variable | Description |
|----------|-------------|
| `K0S_SSH_USER` | SSH username for cluster nodes. Defaults to the command runner (`id -un`) when unset. |
| `K0S_SSH_KEY_PATH` | SSH private key for cluster nodes, used both in the generated k0sctl config and by the post-reset reboots. Defaults to `~/.ssh/id_ed25519`. |
| `K0S_SKIP_REBOOT` | Set to `1` to leave nodes running after `reset` instead of rebooting them. |
| `K0S_REBOOT_TIMEOUT` | Seconds to wait for each node to come back after a reset reboot. Defaults to `600`. |

```bash
K0S_SSH_USER=ubuntu ./create_cluster.sh prd config
```

## Usage

```bash
./create_cluster.sh <prd|sandbox> <command>
```

| Command | Description |
|---------|-------------|
| `bootstrap` | Create a cluster: k0sctl apply without CNI wait → kubeconfig → Helmfile |
| `upgrade` | Upgrade an existing cluster with node, Cilium, and storage health checks before and after k0sctl apply |
| `apply` | Disabled legacy command; select `bootstrap` or `upgrade` |
| `reset` | Reset the cluster: k0sctl reset, then reboot every node and wait for it to return |
| `kubeconfig` | Write kubeconfig to `~/.kube/<env>.yaml` |
| `helmfile` | Apply Helmfile only (requires kubeconfig to exist) |
| `smoke-test` | Test networking, the default PVC class, and each enabled explicit storage test |
| `config` | Print the bootstrap k0sctl config to stdout (dry-run inspection) |
| `upgrade-config` | Print the upgrade k0sctl config to stdout (dry-run inspection) |

### Examples

```bash
# Inspect the generated config
./create_cluster.sh prd config

# Build a new cluster
./create_cluster.sh prd bootstrap

# Inspect and run an existing cluster upgrade
./create_cluster.sh sandbox upgrade-config
./create_cluster.sh sandbox upgrade

# Re-apply Helmfile only
./create_cluster.sh prd helmfile

# Reset the cluster and reboot every node
./create_cluster.sh sandbox reset

# Reset without rebooting
K0S_SKIP_REBOOT=1 ./create_cluster.sh sandbox reset

# Remove stale host keys after recreating all cluster VMs
./remove-known-hosts.sh sandbox
```

Kubeconfig is written to `~/.kube/<env>.yaml`.

### Bootstrap and upgrade safety

Helmfile installs the custom CNI after k0s starts. `bootstrap` therefore sets
`spec.options.wait.enabled: false`; waiting for Ready before Cilium would deadlock.

`upgrade` is intentionally separate and requires an existing kubeconfig. It
uses the following safety controls:

- verifies that all nodes, Cilium, and every enabled storage provider are healthy
  before changing any host;
- enables k0sctl's per-worker Ready wait;
- limits worker disruption to 10 percent and sets the drain timeout to 20
  minutes so workloads can terminate and reschedule cleanly;
- repeats node and storage health checks after k0sctl and Helmfile complete.

OpenEBS requires all namespace pods Ready; NFS requires controller and node
rollouts. The workflow rejects multiple default StorageClasses and verifies the
configured default after Helmfile. Do not bootstrap with `upgrade` or bypass a
failed storage check with `--no-drain`.

With NFS, `smoke-test` verifies an explicit RWX claim across a remount. Cleanup
removes the retained PV object but leaves its TrueNAS directory for inspection.

### Reset

`k0sctl reset` leaves Cilium interfaces, nftables rules and kubelet bind mounts.
`reset` reboots every node so the next `bootstrap` starts cleanly.

Reboots start on all workers, then controllers, and complete in parallel. A new
`boot_id` confirms reboot; SSH alone cannot distinguish a pre-reboot daemon.
The run waits for every reachable node and reports all failures.

Running-cluster reboots belong to Ansible (ADR-0032), which drains first. A
post-reset reboot needs no drain because the cluster no longer exists.

After recreating VMs at the same addresses, run `remove-known-hosts.sh` before
bootstrap. It removes all environment node keys, including hashed entries. Use
`KNOWN_HOSTS_FILE` to select another file.

## Cluster Architecture

- **Datastore**: kine (single controller) or etcd (multiple controllers — count must be odd for quorum); selected automatically based on `K0S_CONTROLLER_ADDRESSES`
- **CNI**: Cilium v1.19.x with kube-proxy replacement and L2 LoadBalancer.
  Envoy Gateway owns ingress (ADR-0011). Worker segment labels restrict L2
  announcements to the LoadBalancer pool's network.
- **Storage CSI**: any configured combination of OpenEBS v4.5.1 LocalPV and
  NFS CSI v4.13.4. OpenEBS uses the SSD mounted at
  `/srv/storage/volume`; NFS uses the environment-specific external export.
- **GPU**: AMD device plugin when GPU workers exist; nodes use label `gpu=amd`
  and taint `gpu=amd:NoSchedule`.
- **CoreDNS**: k0s calculates replicas from Linux nodes. A CoreDNS-only GPU
  toleration permits distribution without admitting other workloads.
