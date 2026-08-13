# eso — External Secrets Operator

Deploys ESO and an OpenBao-backed `ClusterSecretStore` through Argo CD.

## Directory Structure

```
eso/
├── Chart.yaml
├── values.yaml               # Default: openbao server URL, path, role, mountPath
├── prd/values.yaml           # mountPath: kubernetes
├── sandbox/values.yaml       # mountPath: kubernetes-sandbox
└── templates/
    ├── auth-delegator.yaml        # ClusterRoleBinding for ESO TokenReview access
    └── cluster-secret-store.yaml  # ClusterSecretStore: openbao
```

## How It Works

1. The upstream chart installs ESO.
2. `ClusterSecretStore/openbao` uses Kubernetes authentication.
3. `system:auth-delegator` lets OpenBao validate ESO tokens.
4. Workloads reference the store through `ExternalSecret` resources.

## Values

| Key | Default | Description |
|-----|---------|-------------|
| `openbao.server` | `https://openbao.home.butaco.net` | OpenBao API URL |
| `openbao.path` | `secret` | KV v2 mount path |
| `openbao.role` | `k8s-eso` | OpenBao Kubernetes auth role |
| `openbao.mountPath` | `kubernetes` | Kubernetes auth mount path in OpenBao |

Each environment overrides `mountPath` through its Argo CD value file.

After rebuilding a cluster, re-register its CA with OpenBao; see
[`ADR-0012`](../../docs/adr/0012-openbao-eso-cluster-rebuild-registration.md).

## Dependencies

- OpenBao must be configured before ESO can sync secrets.
- The `ClusterSecretStore` syncs at ArgoCD sync-wave `1` (after ESO CRDs are ready).

## Usage

App of Apps passes the environment override (ADR-0014):

```yaml
source:
  path: k8s/eso
  helm:
    valueFiles:
      - prd/values.yaml  # per-env override (openbao.mountPath)
```
