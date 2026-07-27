# lemonade-server

[Lemonade](https://github.com/lemonade-sdk/lemonade) LLM inference server deployed on the prd cluster with AMD GPU (Vulkan) support. Managed by ArgoCD.

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

## GPU / Vulkan

Requires one AMD GPU (`amd.com/gpu: "1"`), allocated by the ROCm k8s-device-plugin (it manages the device even though the backend used here is Vulkan, not ROCm/HIP). The node must be labeled `gpu: amd` and tainted `gpu=amd:NoSchedule`.

Uses the **upstream image** (`ghcr.io/lemonade-sdk/lemonade-server`) with the `vulkan` llamacpp backend, **not** the built-in `rocm` backend.

### Why Vulkan instead of ROCm

lemonade v10.8.0 mis-detects this GPU. Its `llamacpp:rocm` backend reports `Unsupported GPU: gfx1200`: lemonade reads the card's KFD ISA name (`RADV GFX1200`), extracts the raw token `gfx1200`, and compares it for **exact equality** against the wildcard `gfx120X` its recipe table expects — so RDNA4 discrete GPUs (RX 9060 XT) are wrongly rejected. This is an upstream bug; switching the ROCm channel (stable/nightly) does **not** fix it.

The `llamacpp:vulkan` backend has **no GPU-family gate** and needs no custom image: lemonade downloads the matching upstream llama.cpp Vulkan release binary automatically, and the RADV (Mesa) driver already on the host picks up the GPU via the same `/dev/dri` render node the ROCm device plugin injects. Set via `llamacpp.backend=vulkan` in `config.json` on the `lemonade-recipe` PVC, done by the `set-llamacpp-backend` initContainer in the Deployment.

Benchmarking against the previous ROCm-backend setup (a custom image bundling a ROCm-enabled `llama-server`, since retired) showed Vulkan ~20% faster single-request and ~47% faster at 4 concurrent requests on this GPU — consistent with known RDNA4 ROCm/Vulkan gaps upstream (e.g. [ggml-org/llama.cpp#20934](https://github.com/ggml-org/llama.cpp/issues/20934)), not just an artifact of the gfx1200 workaround.

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
