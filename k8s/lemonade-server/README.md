# lemonade-server

[Lemonade](https://github.com/lemonade-sdk/lemonade) LLM inference server deployed on the prd cluster with AMD GPU (ROCm) support. Managed by ArgoCD.

## Directory Structure

```
lemonade-server/
├── values.yaml           # Environment-level overrides (hostname)
└── chart/                # Custom Helm chart
    ├── Chart.yaml
    ├── values.yaml       # Default chart values
    └── templates/
        ├── deployment.yaml  # Recreate strategy; AMD GPU resource requests
        ├── pvc.yaml         # HuggingFace cache, llama.cpp binaries, recipe cache
        ├── service.yaml     # ClusterIP on port 13305
        └── httproute.yaml   # HTTPRoute → shared-gateway-envoy
```

## Access

| Client | Address |
|--------|---------|
| External | `https://lemonade.prd.butaco.net` |
| In-cluster | `http://lemonade-server.lemonade-server.svc.cluster.local:13305` |

## GPU

Requires one AMD GPU (`amd.com/gpu: "1"`). The node must be labeled `gpu: amd` and tainted `gpu=amd:NoSchedule`.

Uses `ghcr.io/lemonade-sdk/lemonade-server` with `LEMONADE_LLAMACPP=rocm` to enable ROCm acceleration.

The image itself contains no ROCm: on the default `stable` ROCm channel lemonade downloads the llama.cpp ROCm build **and its own TheRock ROCm runtime** at first start, into the `lemonade-llama` PVC. It would reuse a system ROCm found via `ROCM_PATH` / `/opt/rocm`, but neither exists in this container, so the runtime is fully self-contained and **independent of the host ROCm version** (`ansible/roles/rocm`). Only the host amdgpu kernel driver has to stay within AMD's [KMD/UMD skew window](https://rocm.docs.amd.com/projects/install-on-linux/en/latest/reference/user-kernel-space-compat-matrix.html) (one year since ROCm 6.4). RDNA4 / `gfx1200` is a supported target on both ROCm channels.

Because the runtime is cached in the PVC, a host ROCm/driver upgrade never invalidates it automatically. If the GPU stops being detected after such an upgrade, force a re-download:

```bash
kubectl -n lemonade-server scale deploy/lemonade-server --replicas=0
# then delete the lemonade-llama PVC and let ArgoCD recreate it
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
| `replicaCount` | `0` | Set to `1` to start (default off to save GPU) |
| `image.tag` | `v10.8.0` | Lemonade server image tag |
| `storage.storageClassName` | `openebs-hostpath` | Storage class |
