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

1. ESO is installed from the upstream `external-secrets` Helm chart (included as dependency).
2. A `ClusterSecretStore` named `openbao` is created, pointing to the OpenBao server using Kubernetes auth.
3. The ESO ServiceAccount is bound to `system:auth-delegator`, allowing OpenBao to validate Kubernetes tokens using the client's login token.
4. All other charts in the cluster use `ExternalSecret` resources referencing the `openbao` ClusterSecretStore.

## Values

| Key | Default | Description |
|-----|---------|-------------|
| `openbao.server` | `https://openbao.home.butaco.net` | OpenBao API URL |
| `openbao.path` | `secret` | KV v2 mount path |
| `openbao.role` | `k8s-eso` | OpenBao Kubernetes auth role |
| `openbao.mountPath` | `kubernetes` | Kubernetes auth mount path in OpenBao |

Each environment overrides `mountPath` through its Argo CD value file.

After rebuilding a cluster, re-register its new CA with OpenBao. See
[`ADR-0012`](../../docs/adr/0012-openbao-eso-cluster-rebuild-registration.md)
and the OpenBao registration runbook in `ansible/README.md`.

## Dependencies

- OpenBao must be deployed and configured before ESO can sync secrets.
  See `ansible/roles/openbao/README.md` for setup steps.
- The `ClusterSecretStore` syncs at ArgoCD sync-wave `1` (after ESO CRDs are ready).

## Usage

App of Apps enables ESO per environment and passes its override (ADR-0014):

```yaml
source:
  path: k8s/eso
  helm:
    valueFiles:
      - prd/values.yaml  # per-env override (openbao.mountPath)
```
