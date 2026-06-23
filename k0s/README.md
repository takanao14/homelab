# k0s Cluster Management

Scripts for managing the k0s cluster lifecycle using k0sctl and Helmfile.

## Prerequisites

| Tool | Purpose |
|------|---------|
| `k0sctl` | Cluster setup / reset |
| `helmfile` / `helm` | Helm deployments for CNI, storage, and device plugins |
| `kubectl` | Apply Gateway API CRDs |
| `cilium` CLI | Wait for Cilium to become ready |
| `sops` | Decrypt secrets files |

## Directory Structure

```
k0s/
├── create_cluster.sh              # Entry point: ./create_cluster.sh <dev|prd|sandbox> <command>
├── template_lib.sh                # Shared library: k0sctl config generation and cluster management logic
├── helmfile.yaml.gotmpl           # Default Helm release definitions (cilium / openebs or longhorn / cilium-config)
├── env/
│   ├── dev.sh                     # Dev non-secret variables (committed)
│   ├── prd.sh                     # Prd non-secret variables (committed)
│   └── sandbox.sh                 # Sandbox non-secret variables (committed)
├── secrets.dev.enc.env            # SOPS-encrypted secrets for dev (committed)
├── secrets.prd.enc.env            # SOPS-encrypted secrets for prd (committed)
├── secrets.sandbox.enc.env        # SOPS-encrypted secrets for sandbox (committed)
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

Variables are split between plain `env/` files (non-secrets) and SOPS-encrypted `secrets.*.enc.env` files (secrets).

### Non-secret (`env/dev.sh` / `env/prd.sh` / `env/sandbox.sh`)

| Variable | Description |
|----------|-------------|
| `K0S_CONTROLLER_ADDRESSES` | Comma-separated controller node IP addresses |
| `K0S_WORKER_ADDRESSES` | Comma-separated worker node IP addresses |
| `K0S_GPU_WORKER_ADDRESSES` | Comma-separated GPU worker IP addresses (optional; omit for no GPU workers) |
| `K0S_LB_POOL` | Cilium LoadBalancer IP pool range (`start,stop`) |
| `K0S_VERSION` | k0s version to install (optional; omits `version:` if unset) |
| `K0S_STORAGE_PROVIDER` | Storage CSI to deploy: `openebs` (default) or `longhorn` |

### Secrets (`secrets.dev.enc.env` / `secrets.prd.enc.env` / `secrets.sandbox.enc.env`)

| Variable | Description |
|----------|-------------|
| `K0S_SSH_USER` | SSH username for cluster nodes |

Edit secrets with:

```bash
sops edit secrets.dev.enc.env
sops edit secrets.prd.enc.env
sops edit secrets.sandbox.enc.env
```

## Usage

```bash
./create_cluster.sh <dev|prd|sandbox> <command>
```

| Command | Description |
|---------|-------------|
| `apply` | Full setup: k0sctl apply → fetch kubeconfig → Gateway API CRDs → helmfile apply |
| `reset` | Reset the cluster: k0sctl reset |
| `kubeconfig` | Write kubeconfig to `~/.kube/<env>.yaml` |
| `helmfile` | Apply Helmfile only (requires kubeconfig to exist) |
| `gateway-api` | Apply Gateway API CRDs only (requires kubeconfig to exist) |
| `config` | Print the generated k0sctl config to stdout (dry-run inspection) |

### Examples

```bash
# Inspect the generated config
./create_cluster.sh dev config

# Build a new dev cluster
./create_cluster.sh dev apply

# Re-apply Helmfile only
./create_cluster.sh dev helmfile

# Reset the cluster
./create_cluster.sh dev reset
```

Kubeconfig is written to `~/.kube/<env>.yaml` (e.g. `~/.kube/dev.yaml`, `~/.kube/prd.yaml`).

## Cluster Architecture

- **Datastore**: kine (single controller) or etcd (multiple controllers — count must be odd for quorum); selected automatically based on `K0S_CONTROLLER_ADDRESSES`
- **CNI**: Cilium v1.19.3 (kube-proxy disabled, L2 LoadBalancer, Gateway API enabled)
- **Storage CSI**: OpenEBS v4.4.0 LocalPV or Longhorn v1.11.1 — selected via `K0S_STORAGE_PROVIDER`; both use SSD mounted at `/srv/storage/volume`
- **GPU**: AMD GPU Device Plugin (enabled when `K0S_GPU_WORKER_ADDRESSES` is set; nodes labeled `gpu=amd` and tainted `gpu=amd:NoSchedule`)
- **CoreDNS**: Replica count is calculated automatically by k0s from the number of Linux nodes. When GPU workers are configured, `template_lib.sh` adds a CoreDNS-only toleration for `gpu=amd:NoSchedule`, allowing CoreDNS replicas to be distributed across standard and GPU workers without making other workloads eligible for GPU workers.
