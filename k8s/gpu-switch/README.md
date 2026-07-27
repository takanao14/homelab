# gpu-switch

Authenticated web UI for switching the single AMD GPU between the prd
workloads. The backend discovers switchable Deployments by label, scales every
one to zero, and then scales the selected target to one. See
[ADR-0027](../../docs/adr/0027-gpu-workload-switching-web-ui.md).

The app-of-apps chart deploys gpu-switch to prd at sync wave 1, after ESO and
the shared Gateway. It remains disabled in sandbox.

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

Kubernetes RBAC cannot constrain `deployments/scale` by label. Therefore
`chart/values.yaml` contains the one explicit `gpuWorkloads` list. Keep it
aligned with the labelled Deployments. The template groups entries by namespace
and creates one Role and RoleBinding per namespace.

The current write targets are:

- `comfyui/comfyui`
- `lemonade-server/lemonade-server`
- `lemonade-server/lemonade-server-rocm`
- `ollama/ollama`
- `vllm/vllm`

## Authentication

Envoy Gateway enforces Basic Auth through a `SecurityPolicy` targeting this
chart's `HTTPRoute`. ESO reads the htpasswd content from:

```text
secret/k8s/gpu-switch/basic-auth
```

The KV property is `htpasswd`, projected into Secret key `.htpasswd`. The value
must use the `{SHA}` htpasswd format required by Envoy Gateway. OpenBao policy
and secret provisioning are implemented separately in Phase 4.

## Image release

The Deployment uses:

```text
ghcr.io/takanao14/homelab/gpu-switch:0.1.0
```

The public package requires no `imagePullSecret`. Image releases are immutable
and triggered by tags such as `gpu-switch/v0.1.0`; Renovate updates the tag in
`chart/values.yaml`.

## Render

Render with the same common and environment values that the future Argo CD
Application will reference:

```bash
helm lint k8s/gpu-switch/chart \
  -f k8s/gpu-switch/values.yaml \
  -f k8s/gpu-switch/prd/values.yaml

helm template gpu-switch k8s/gpu-switch/chart \
  --namespace gpu-switch \
  -f k8s/gpu-switch/values.yaml \
  -f k8s/gpu-switch/prd/values.yaml
```

After Argo CD registration, verify the live value-file contract before changing
values:

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
