# lemonade-server

[Lemonade](https://github.com/lemonade-sdk/lemonade) LLM inference server deployed on the prd cluster with AMD GPU (Vulkan and ROCm) support. Managed by ArgoCD.

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

Requires one AMD GPU (`amd.com/gpu: "1"`), allocated by the ROCm k8s-device-plugin (it manages the device even though the backend used here is Vulkan, not ROCm/HIP). The node must be labeled `gpu: amd` and tainted `gpu=amd:NoSchedule`.

Uses the **upstream image** (`ghcr.io/lemonade-sdk/lemonade-server`) with the `vulkan` llamacpp backend, **not** the built-in `rocm` backend.

### Why Vulkan instead of ROCm

Decision and rejected alternatives: [ADR-0028](../../docs/adr/0028-lemonade-vulkan-backend-over-rocm-on-rdna4.md).

lemonade v10.8.0 mis-detects this GPU. Its `llamacpp:rocm` backend reports `Unsupported GPU: gfx1200`: lemonade reads the card's KFD ISA name (`RADV GFX1200`), extracts the raw token `gfx1200`, and compares it for **exact equality** against the wildcard `gfx120X` its recipe table expects — so RDNA4 discrete GPUs (RX 9060 XT) are wrongly rejected. This is an upstream bug; switching the ROCm channel (stable/nightly) does **not** fix it.

The `llamacpp:vulkan` backend has **no GPU-family gate** and needs no custom image: lemonade downloads the matching upstream llama.cpp Vulkan release binary automatically, and the RADV (Mesa) driver already on the host picks up the GPU via the same `/dev/dri` render node the ROCm device plugin injects. Set via `llamacpp.backend=vulkan` in `config.json` on the `lemonade-recipe` PVC, done by the `set-llamacpp-backend` initContainer in the Deployment.

Benchmarking against the ROCm-backend setup below showed Vulkan ~20% faster single-request and ~47% faster at 4 concurrent requests on this GPU — consistent with known RDNA4 ROCm/Vulkan gaps upstream (e.g. [ggml-org/llama.cpp#20934](https://github.com/ggml-org/llama.cpp/issues/20934)), not just an artifact of the gfx1200 workaround.

### ROCm serving and benchmark variant

`lemonade-server-rocm` is a second Deployment used both as an MXFP4 serving path and to re-run backend comparisons (e.g. after a lemonade or ROCm upgrade). It uses the same custom image the primary Deployment used before the Vulkan switch (`forgejo.home.butaco.net/takanao/lemonade-docker`, built from [`lemonade-docker`](https://forgejo.home.butaco.net/takanao/lemonade-docker)), which bakes in a ROCm-enabled `llama-server` and drives it via the `system` backend (see `chart/templates/deployment-rocm.yaml`).

The MXFP4 measurement in [ADR-0029](../../docs/adr/0029-rocm-serving-path-for-mxfp4-models.md) is concurrency-dependent: Vulkan generated a single request 11.2% faster, while ROCm delivered 21.1% more aggregate completion throughput with four concurrent requests. `opencode.json` currently selects the ROCm endpoint; retain both Deployments because neither backend wins both loads.

It shares the `lemonade-huggingface`/`lemonade-llama`/`lemonade-recipe` PVCs with the primary Deployment, is labelled `homelab/gpu-switchable: "true"` like every other GPU workload, and defaults to `replicaCount: 0`. Start it the same way as any other GPU workload:

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
