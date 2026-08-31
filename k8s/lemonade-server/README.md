# lemonade-server

[Lemonade](https://github.com/lemonade-sdk/lemonade) Vulkan and ROCm inference on prd.

## Directory Structure

```
lemonade-server/
├── values.yaml           # Environment-level overrides (hostname)
└── chart/                # Custom Helm chart
    ├── Chart.yaml
    ├── values.yaml       # Default chart values
    └── templates/
        ├── deployment*.yaml  # Vulkan and ROCm variants
        ├── pvc.yaml          # Shared caches
        ├── service*.yaml
        └── httproute*.yaml
```

## Access

| Client | Address |
|--------|---------|
| External | `https://lemonade.prd.butaco.net` |
| In-cluster | `http://lemonade-server.lemonade-server.svc.cluster.local:13305` |
| External (ROCm) | `https://lemonade-rocm.prd.butaco.net` |
| In-cluster (ROCm) | `http://lemonade-server-rocm.lemonade-server.svc.cluster.local:13305` |

## GPU backends

Both backends request one `amd.com/gpu`. The primary uses upstream llamacpp
`vulkan`; the comparison variant uses the custom ROCm system backend.

### Why Vulkan instead of ROCm

Decision and rejected alternatives: [ADR-0028](../../docs/adr/0028-lemonade-vulkan-backend-over-rocm-on-rdna4.md).

Lemonade v10.8.0 rejects gfx1200 through a faulty `gfx120X` family check.
Vulkan avoids this gate, uses plugin-injected RADV devices, and is pinned in the
recipe PVC by an init container.

### ROCm serving and benchmark variant

`lemonade-server-rocm` retains the custom ROCm `llama-server` for MXFP4 and
repeatable comparisons ([ADR-0029](../../docs/adr/0029-rocm-serving-path-for-mxfp4-models.md)).

`rocm.image.tag` is the `llamacpp-rocm` build number baked into the image by the
separate `lemonade-docker` repository. `b1303` (2026-08-01) moved those builds
from ROCm 7.15 to ROCm 10.1, so `b1319` bundles `10.1.0a20260822` — one minor
ahead of the ROCm 10.0 host stack from `ansible/roles/rocm`. Rebuild and push
the image there before bumping this tag, then re-verify GPU offload; ADR-0029's
benchmark numbers were taken on `b1302` (ROCm 7.15) and need re-measuring.

gpu-switch runs only one variant, making their shared PVCs safe. Both default
to zero replicas:

```bash
scripts/gpu-switch.sh lemonade-server-rocm   # stops every other GPU workload, starts this one
scripts/gpu-switch.sh status                 # confirm it's running
scripts/gpu-switch.sh ollama                 # switch back when done
```

### Verifying GPU offload

```bash
kubectl -n lemonade-server scale deploy/lemonade-server --replicas=1
kubectl -n lemonade-server exec deploy/lemonade-server -- ./lemonade backends | grep -i llamacpp
# expect: llamacpp vulkan -> usable, no "Unsupported GPU"
kubectl -n lemonade-server logs deploy/lemonade-server | grep -iE "vulkan|RADV|gfx"
```

## Storage

| PVC | Default Size | Mount Path |
|-----|-------------|------------|
| `lemonade-huggingface` | 90Gi | `/root/.cache/huggingface` |
| `lemonade-llama` | 5Gi | `/opt/lemonade/llama` |
| `lemonade-recipe` | 5Gi | `/root/.cache/lemonade` |

## Key Values

| Key | Default | Description |
|-----|---------|-------------|
| `hostname` | `lemonade.prd.butaco.net` | HTTPRoute hostname |
| `gateway.timeouts.request` | unset (chart) / `10m` (`values.yaml`) | End-to-end HTTPRoute timeout for long LLM responses |
| `gateway.timeouts.backendRequest` | unset (chart) / `10m` (`values.yaml`) | Gateway-to-Lemonade request timeout |
| `replicaCount` | `0` | Started through gpu-switch |
| `image.repository` | `ghcr.io/lemonade-sdk/lemonade-server` | Upstream image |
| `image.tag` | `v10.8.0` | Lemonade version |
| `storage.storageClassName` | `openebs-hostpath` | Storage class |
| `rocm.replicaCount` | `0` | ROCm variant, started through gpu-switch |
| `rocm.hostname` | `lemonade-rocm.prd.butaco.net` | HTTPRoute hostname for the ROCm variant |
| `rocm.image.repository` | `forgejo.home.butaco.net/takanao/lemonade-docker` | Custom ROCm image |
| `rocm.image.tag` | `b1319` | Bundled `llamacpp-rocm` build number (ROCm 10.1) |
