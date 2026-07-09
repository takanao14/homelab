# k0s Cluster Management

Scripts for managing the k0s cluster lifecycle using k0sctl and Helmfile.

## Prerequisites

| Tool | Purpose |
|------|---------|
| `k0sctl` | Cluster setup / reset |
| `helmfile` / `helm` | Helm deployments for CNI, storage, and device plugins |
| `kubectl` | Cluster readiness checks and helmfile hooks |
| `cilium` CLI | Wait for Cilium to become ready |
## Directory Structure

```
k0s/
├── create_cluster.sh              # Entry point: ./create_cluster.sh <env> <command>
├── template_lib.sh                # Shared library: k0sctl config generation and cluster management logic
├── helmfile.yaml.gotmpl           # Default Helm release definitions (cilium / openebs or longhorn / cilium-config)
├── env/
│   ├── prd.sh                     # Prd non-secret variables (committed)
│   └── sandbox.sh                 # Sandbox non-secret variables (committed)
├── charts/
│   └── cilium-config/             # Local chart for Cilium L2 policy and IP pool
├── values/
│   ├── amd-device-plugin.yaml     # AMD GPU Device Plugin Helm values
│   ├── cilium.yaml.gotmpl         # Cilium Helm values
│   ├── cilium-config.yaml.gotmpl  # cilium-config Helm values (IP pool range)
│   ├── openebs.yaml               # OpenEBS Helm values
│   └── longhorn.yaml              # Longhorn Helm values
├── hook/
│   ├── ssdsetup.sh                # Format and mount SSD on worker node
│   └── mirror.sh                  # Configure containerd docker.io mirror
└── scripts/
    └── wait-cilium-crds.sh        # Helmfile presync hook: wait for Cilium CRDs
```

## Environment Variables

Cluster topology and non-secret settings live in `env/` files. `K0S_SSH_USER` can be provided as an environment variable; when it is unset, `create_cluster.sh` uses the user running the command (`id -un`).

### Environment files (`env/prd.sh` / `env/sandbox.sh`)

| Variable | Description |
|----------|-------------|
| `K0S_CONTROLLER_ADDRESSES` | Comma-separated controller node IP addresses |
| `K0S_WORKER_ADDRESSES` | Comma-separated worker node IP addresses |
| `K0S_GPU_WORKER_ADDRESSES` | Comma-separated GPU worker IP addresses (optional; omit for no GPU workers) |
| `K0S_LB_POOL` | Cilium LoadBalancer IP pool range (`start,stop`) |
| `K0S_VERSION` | k0s version to install (optional; omits `version:` if unset) |
| `K0S_STORAGE_PROVIDER` | Storage CSI to deploy: `openebs` (default) or `longhorn` |

### Optional shell variables

| Variable | Description |
|----------|-------------|
| `K0S_SSH_USER` | SSH username for cluster nodes. Defaults to the command runner (`id -un`) when unset. |

```bash
K0S_SSH_USER=ubuntu ./create_cluster.sh prd config
```

## Usage

```bash
./create_cluster.sh <prd|sandbox> <command>
```

| Command | Description |
|---------|-------------|
| `apply` | Full setup: k0sctl apply → fetch kubeconfig → helmfile apply |
| `reset` | Reset the cluster: k0sctl reset |
| `kubeconfig` | Write kubeconfig to `~/.kube/<env>.yaml` |
| `helmfile` | Apply Helmfile only (requires kubeconfig to exist) |
| `config` | Print the generated k0sctl config to stdout (dry-run inspection) |

### Examples

```bash
# Inspect the generated config
./create_cluster.sh prd config

# Build a cluster
./create_cluster.sh prd apply

# Re-apply Helmfile only
./create_cluster.sh prd helmfile

# Reset the cluster
./create_cluster.sh sandbox reset
```

Kubeconfig is written to `~/.kube/<env>.yaml` (e.g. `~/.kube/prd.yaml`, `~/.kube/sandbox.yaml`).

## Cluster Architecture

- **Datastore**: kine (single controller) or etcd (multiple controllers — count must be odd for quorum); selected automatically based on `K0S_CONTROLLER_ADDRESSES`
- **CNI**: Cilium v1.19.x (kube-proxy disabled, L2 LoadBalancer; ingress/Gateway API controllers disabled — shared ingress is Envoy Gateway, ArgoCD-managed, see ADR-0011). Workers are labeled `homelab/l2-segment=<first-three-IP-octets>` by k0s install flags and re-synced before Helmfile runs, so L2 announcements only run on nodes in the LoadBalancer pool's segment.
- **Storage CSI**: OpenEBS v4.4.0 LocalPV or Longhorn v1.11.1 — selected via `K0S_STORAGE_PROVIDER`; both use SSD mounted at `/srv/storage/volume`
- **GPU**: AMD GPU Device Plugin (enabled when `K0S_GPU_WORKER_ADDRESSES` is set; GPU workers are labeled `gpu=amd` and tainted `gpu=amd:NoSchedule`)
- **CoreDNS**: Replica count is calculated automatically by k0s from the number of Linux nodes. When GPU workers are configured, `template_lib.sh` adds a CoreDNS-only toleration for `gpu=amd:NoSchedule`, allowing CoreDNS replicas to be distributed across standard and GPU workers without making other workloads eligible for GPU workers.
