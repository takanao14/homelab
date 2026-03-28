# k0s Cluster Management

Scripts for managing the k0s cluster lifecycle using k0sctl and Helmfile.

## Prerequisites

| Tool | Purpose |
|------|---------|
| `k0sctl` | Cluster setup / reset |
| `helmfile` / `helm` | Helm deployments for CNI and storage |
| `kubectl` | Apply Gateway API CRDs |
| `cilium` CLI | Wait for Cilium to become ready |
| `envsubst` | Expand variables in the k0sctl config template |
| `sops` | Decrypt secrets files |
| `direnv` | Auto-load environment variables |

## Directory Structure

```
k0s/
├── Makefile                       # Entry point and all cluster management logic
├── k0sctl.tmpl.yaml               # k0sctl config template (expanded with envsubst)
├── helmfile.yaml                  # Helm release definitions (cilium / openebs / cilium-config)
├── .env.dev                       # Dev non-secret variables (gitignored)
├── .env.prd                       # Prd non-secret variables (gitignored)
├── secrets.dev.enc.env            # SOPS-encrypted secrets for dev (committed)
├── secrets.prd.enc.env            # SOPS-encrypted secrets for prd (committed)
├── charts/
│   └── cilium-config/             # Local chart for Cilium L2 policy and IP pool
├── values/
│   ├── cilium.yaml.gotmpl         # Cilium Helm values
│   ├── cilium-config.yaml.gotmpl  # cilium-config Helm values (IP pool range)
│   └── openebs.yaml               # OpenEBS Helm values
├── hook/
│   ├── ssdsetup.sh                # Format and mount SSD on worker node
│   └── mirror.sh                  # Configure containerd docker.io mirror
└── test/
    ├── default-openebs.yaml       # Smoke test for OpenEBS default StorageClass
    └── load-balancer.yaml         # Smoke test for LoadBalancer Service
```

## Environment Variables

Variables are split between plain `.env.*` files (non-secrets) and SOPS-encrypted `secrets.*.enc.env` files (secrets). Both are sourced automatically by `create_cluster.sh`.

### Non-secret (`.env.dev` / `.env.prd`)

| Variable | Description |
|----------|-------------|
| `K0S_CONTROLLER_ADDRESS` | Controller node IP address |
| `K0S_WORKER_ADDRESS` | Worker node IP address |
| `K0S_LB_POOL` | Cilium LoadBalancer IP pool range (`start,stop`) |

### Secrets (`secrets.dev.enc.env` / `secrets.prd.enc.env`)

| Variable | Description |
|----------|-------------|
| `K0S_SSH_USER` | SSH username for cluster nodes |

Edit secrets with:

```bash
sops edit secrets.dev.enc.env
sops edit secrets.prd.enc.env
```

## Usage

```bash
make ENV=<dev|prd> <target>
```

| Target | Description |
|--------|-------------|
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
make ENV=dev config

# Build a new dev cluster
make ENV=dev apply

# Re-apply Helmfile only
make ENV=dev helmfile

# Reset the cluster
make ENV=dev reset
```

Kubeconfig is written to `~/.kube/dev.yaml` or `~/.kube/prd.yaml`.

## Cluster Architecture

- **Datastore**: kine (etcd replacement, suited for single-node control plane)
- **CNI**: Cilium (kube-proxy disabled, L2 LoadBalancer, Gateway API enabled)
- **Storage CSI**: OpenEBS LocalPV (uses SSD mounted at `/srv/storage/volume`)
