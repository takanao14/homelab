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
        ├── deployment.yaml       # Recreate strategy; AMD GPU resource requests; vulkan backend
        ├── deployment-rocm.yaml  # Same, but the custom ROCm image + system backend (see below)
        ├── pvc.yaml              # HuggingFace cache, llama.cpp binaries, recipe cache
        ├── service.yaml          # ClusterIP on port 13305
        ├── service-rocm.yaml     # ClusterIP for the rocm Deployment
        ├── httproute.yaml        # HTTPRoute → shared-gateway-envoy
        └── httproute-rocm.yaml   # HTTPRoute for the rocm Deployment
```

## Access

| Client | Address |
|--------|---------|
| External | `https://lemonade.prd.butaco.net` |
| In-cluster | `http://lemonade-server.lemonade-server.svc.cluster.local:13305` |
| External (ROCm) | `https://lemonade-rocm.prd.butaco.net` |
| In-cluster (ROCm) | `http://lemonade-server-rocm.lemonade-server.svc.cluster.local:13305` |

## GPU backends

Both backends request one `amd.com/gpu` on the labelled and tainted GPU node.

The primary uses the upstream image with llamacpp `vulkan`, not built-in `rocm`.

### Why Vulkan instead of ROCm

Decision and rejected alternatives: [ADR-0028](../../docs/adr/0028-lemonade-vulkan-backend-over-rocm-on-rdna4.md).

Lemonade v10.8.0's built-in ROCm recipe compares gfx1200 against literal
`gfx120X` and rejects RDNA4. Changing ROCm channels does not fix this upstream bug.

Vulkan has no family gate and uses host RADV through plugin-injected `/dev/dri`.
An init container pins it in the recipe PVC.

Vulkan measured ~20% faster for one request and ~47% faster at four concurrent
requests in the original comparison.

### ROCm serving and benchmark variant

`lemonade-server-rocm` retains the custom ROCm `llama-server` image and `system`
backend for MXFP4 serving and repeatable comparisons.

For MXFP4, Vulkan won one request by 11.2%, while ROCm delivered 21.1% more
four-request throughput ([ADR-0029](../../docs/adr/0029-rocm-serving-path-for-mxfp4-models.md)). Retain both paths.

Both variants share PVCs safely because gpu-switch runs only one. The ROCm
variant is labelled switchable and defaults to zero replicas:

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
| `replicaCount` | `0` | Set to `1` to start (default off to save GPU) |
| `image.repository` | `ghcr.io/lemonade-sdk/lemonade-server` | Upstream image |
| `image.tag` | `v10.8.0` | Lemonade version |
| `storage.storageClassName` | `openebs-hostpath` | Storage class |
| `rocm.replicaCount` | `0` | Set to `1` to start the ROCm serving/benchmark variant (use `gpu-switch.sh` instead in practice) |
| `rocm.hostname` | `lemonade-rocm.prd.butaco.net` | HTTPRoute hostname for the ROCm variant |
| `rocm.image.repository` | `forgejo.home.butaco.net/takanao/lemonade-docker` | Custom ROCm-enabled image for the serving/benchmark variant |
| `rocm.image.tag` | `b1302` | Bundled `llamacpp-rocm` build number |
