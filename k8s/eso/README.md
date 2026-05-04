# eso — External Secrets Operator

Deploys [External Secrets Operator](https://external-secrets.io/) (ESO) and configures a `ClusterSecretStore` backed by OpenBao. Managed by ArgoCD.

## Directory Structure

```
eso/
├── Chart.yaml
├── values.yaml               # Default: openbao server URL, path, role, mountPath
└── templates/
    ├── cluster-secret-store.yaml  # ClusterSecretStore: openbao
    └── token-reviewer.yaml        # ServiceAccount + ClusterRoleBinding for OpenBao Kubernetes auth
```

## How It Works

1. ESO is installed from the upstream `external-secrets` Helm chart (included as dependency).
2. A `ClusterSecretStore` named `openbao` is created, pointing to the OpenBao server using Kubernetes auth.
3. The `openbao-token-reviewer` ServiceAccount (bound to `system:auth-delegator`) allows OpenBao to validate Kubernetes tokens.
4. All other charts in the cluster use `ExternalSecret` resources referencing the `openbao` ClusterSecretStore.

## Values

| Key | Default | Description |
|-----|---------|-------------|
| `openbao.server` | `https://openbao.home.butaco.net` | OpenBao API URL |
| `openbao.path` | `secret` | KV v2 mount path |
| `openbao.role` | `k8s-eso` | OpenBao Kubernetes auth role |
| `openbao.mountPath` | `kubernetes` | Kubernetes auth mount path in OpenBao |

The `mountPath` can be overridden per-environment in the ArgoCD Application. For example, the dev cluster uses `kubernetes-dev`.

## Dependencies

- OpenBao must be deployed and configured before ESO can sync secrets.
  See `ansible/roles/openbao/README.md` for setup steps.
- The `ClusterSecretStore` syncs at ArgoCD sync-wave `1` (after ESO CRDs are ready).

## Usage

```yaml
# In ArgoCD Application (k8s/argocd/{env}/apps/eso.yaml)
source:
  path: k8s/eso
  helm:
    values: |
      openbao:
        mountPath: "kubernetes-dev"  # dev cluster override
```
