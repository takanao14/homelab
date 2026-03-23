# k0s Cluster Management

Scripts for managing the k0s cluster lifecycle using k0sctl and Helmfile.

## Prerequisites

| Tool | Purpose |
|---|---|
| `k0sctl` | Cluster setup / reset |
| `helmfile` / `helm` | Helm deployments for CNI and storage |
| `kubectl` | Apply Gateway API CRDs |
| `cilium` CLI | Wait for Cilium to become ready |
| `envsubst` | Expand variables in the k0sctl config template |

## Directory Structure

```
k0s/
├── create_cluster.sh          # Entry point
├── template_lib.sh            # Core logic (sourced by create_cluster.sh)
├── k0sctl.tmpl.yaml           # k0sctl config template (expanded with envsubst)
├── helmfile.yaml              # Helm release definitions (cilium / openebs / cilium-config)
├── .env.dev                   # dev environment variables (gitignored)
├── .env.dev.sample            # dev environment variables sample
├── .env.prd                   # prd environment variables (gitignored)
├── .env.prd.sample            # prd environment variables sample
├── charts/
│   └── cilium-config/         # Local chart for Cilium L2 policy and IP pool
├── values/
│   ├── cilium.yaml.gotmpl     # Cilium Helm values
│   ├── cilium-config.yaml.gotmpl  # cilium-config Helm values (IP pool range)
│   └── openebs.yaml           # OpenEBS Helm values
├── hook/
│   ├── ssdsetup.sh            # Format and mount SSD on worker node
│   └── mirror.sh              # Configure containerd docker.io mirror
└── test/
    ├── default-openebs.yaml   # Smoke test for OpenEBS default StorageClass
    └── load-balancer.yaml     # Smoke test for LoadBalancer Service
```

## Environment Variables

Defined in `.env.*` files per environment. Automatically sourced by `create_cluster.sh`. Copy from the sample files to get started.

```bash
cp .env.dev.sample .env.dev
cp .env.prd.sample .env.prd
```

| Variable | Description |
|---|---|
| `K0S_SSH_USER` | SSH username |
| `K0S_CONTROLLER_ADDRESS` | Controller node IP address |
| `K0S_WORKER_ADDRESS` | Worker node IP address |
| `K0S_CLUSTER_NAME` | Cluster name (k0sctl metadata) |
| `K0S_LB_POOL_START` | Cilium LoadBalancer IP pool start address |
| `K0S_LB_POOL_STOP` | Cilium LoadBalancer IP pool end address |

## Usage

```bash
./create_cluster.sh <dev|prd> <command>
```

| Command | Description |
|---|---|
| `apply` | Full setup: k0sctl apply → fetch kubeconfig → helmfile apply → Gateway API CRDs |
| `reset` | Reset the cluster: k0sctl reset |
| `kubeconfig` | Write kubeconfig to `~/.kube/<env>.yaml` |
| `helmfile` | Apply Helmfile only (requires kubeconfig to exist) |
| `gateway-api` | Apply Gateway API CRDs only (requires kubeconfig to exist) |
| `config` | Print k0sctl config to stdout (for dry-run inspection) |
| `help` | Show help |

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

Kubeconfig is written to `~/.kube/dev.yaml` or `~/.kube/prd.yaml`.

## Cluster Architecture

- **Datastore**: kine (etcd replacement, suited for single-node control plane)
- **CNI**: Cilium (kube-proxy disabled, L2 LoadBalancer, Gateway API enabled)
- **Storage CSI**: OpenEBS LocalPV (uses SSD mounted at `/srv/storage/volume`)