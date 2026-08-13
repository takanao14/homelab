# gpu-switch

Authenticated UI for assigning the single prd AMD GPU. It discovers labelled
Deployments, scales all down, then starts the selected target. See
[ADR-0027](../../docs/adr/0027-gpu-workload-switching-web-ui.md).

App of Apps deploys it to prd at wave 1; sandbox remains disabled.

## Layout

```text
gpu-switch/
├── app/                    # Go backend and embedded browser UI
├── chart/
│   ├── Chart.yaml
│   ├── values.yaml         # Image, RBAC targets, Gateway and OpenBao defaults
│   └── templates/
├── values.yaml             # Common resource reservations
├── prd/values.yaml         # prd hostname
└── sandbox/values.yaml     # Disabled environment contract
```

## Critical label invariant

**Never add `homelab/gpu-switchable: "true"` to the gpu-switch Deployment or
Pod.** The backend uses that label as its discovery source of truth. Adding it
would make gpu-switch discover itself and scale itself to zero.

The label belongs only on workloads that consume the shared GPU.

## RBAC boundary

The ServiceAccount is intentionally not cluster-admin:

| Scope | Resource | Names | Verbs |
|---|---|---|---|
| Cluster | `deployments` | Label-filtered by the application | `list` |
| Each GPU namespace | `deployments/scale` | Explicit `resourceNames` | `get`, `patch` |

RBAC cannot label-scope `deployments/scale`, so `gpuWorkloads` explicitly
allowlists writes. Keep it aligned with labels; templates create namespace Roles.

The current write targets are:

- `comfyui/comfyui`
- `lemonade-server/lemonade-server`
- `lemonade-server/lemonade-server-rocm`
- `ollama/ollama`
- `vllm/vllm`

## Authentication

Envoy Gateway applies Basic Auth to the HTTPRoute. ESO reads htpasswd from:

```text
secret/k8s/gpu-switch/basic-auth
```

Property `htpasswd` becomes `.htpasswd` and must use Envoy's `{SHA}` format.

## Image release

The Deployment uses:

```text
ghcr.io/takanao14/homelab/gpu-switch:0.1.0
```

The public immutable image needs no pull Secret; Renovate updates its tag.

## Render

Render with the same values used by Argo CD:

```bash
helm lint k8s/gpu-switch/chart \
  -f k8s/gpu-switch/values.yaml \
  -f k8s/gpu-switch/prd/values.yaml

helm template gpu-switch k8s/gpu-switch/chart \
  --namespace gpu-switch \
  -f k8s/gpu-switch/values.yaml \
  -f k8s/gpu-switch/prd/values.yaml
```

Before changing values, verify the live Argo CD value-file contract:

```bash
kubectl -n argocd get application gpu-switch \
  -o jsonpath='{.spec.sources[*].helm.valueFiles}'
```

## Runtime properties

- One replica with `Recreate`; the switch mutex is process-local.
- Non-root distroless container with a read-only root filesystem and all Linux
  capabilities dropped.
- `/healthz` probes process health only and never call the Kubernetes API.
- Initial requests are `10m` CPU and `32Mi` memory, with no limits. Replace
  these with seven-day observations after deployment.
- The sandbox Application remains disabled because that cluster has no GPU
  worker.
