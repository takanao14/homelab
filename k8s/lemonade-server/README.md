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

## GPU / ROCm

Requires one AMD GPU (`amd.com/gpu: "1"`), allocated by the ROCm k8s-device-plugin. The node must be labeled `gpu: amd` and tainted `gpu=amd:NoSchedule`.

Uses a **custom image** (`forgejo.home.butaco.net/takanao/lemonade-docker`, built from [`lemonade-docker`](https://forgejo.home.butaco.net/takanao/lemonade-docker)) with the `system` llamacpp backend, **not** the upstream image's built-in `rocm` backend.

### Why the custom image

lemonade v10.8.0 mis-detects this GPU. Its `llamacpp:rocm` backend reports `Unsupported GPU: gfx1200`: lemonade reads the card's KFD ISA name (`RADV GFX1200`), extracts the raw token `gfx1200`, and compares it for **exact equality** against the wildcard `gfx120X` its recipe table expects — so RDNA4 discrete GPUs (RX 9060 XT) are wrongly rejected. This is an upstream bug; switching the ROCm channel (stable/nightly) does **not** fix it.

The `llamacpp:system` backend has **no GPU-family gate**, so we bypass the bug by:

1. Baking a ROCm-enabled `llama-server` into the image (the `llamacpp-rocm` `gfx120X` build, which bundles its own ROCm 7 runtime — see the `lemonade-docker` repo). It is exposed on `PATH` with `libggml-hip.so` via `LEMONADE_GGML_HIP_PATH` + `ldconfig`.
2. Pinning `llamacpp.backend=system` in `config.json` on the `lemonade-recipe` PVC, done by the `set-llamacpp-backend` initContainer in the Deployment.

The bundled ROCm 7 userspace is **independent of the host ROCm version** (`ansible/roles/rocm`, currently 7.14). Only the host amdgpu kernel driver must stay within AMD's [KMD/UMD skew window](https://rocm.docs.amd.com/projects/install-on-linux/en/latest/reference/user-kernel-space-compat-matrix.html) (one year since ROCm 6.4), which the bundled ROCm 7 satisfies against the 7.14 host driver.

### Verifying GPU offload

```bash
kubectl -n lemonade-server scale deploy/lemonade-server --replicas=1
kubectl -n lemonade-server exec deploy/lemonade-server -- ./lemonade backends | grep -i llamacpp
# expect: llamacpp system -> usable, no "Unsupported GPU"
kubectl -n lemonade-server logs deploy/lemonade-server | grep -iE "ROCm|gfx|HIP|offload"
```

### Upstream fix

If lemonade fixes the gfx1200 detection (`identify_rocm_arch_from_name` / `device_matches_constraint` in `system_info.cpp`), the built-in `rocm` backend becomes usable and this custom image can be retired in favour of the upstream image + `llamacpp.backend=rocm`.

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
| `image.repository` | `forgejo.home.butaco.net/takanao/lemonade-docker` | Custom ROCm-enabled image |
| `image.tag` | `b1302` | Bundled `llamacpp-rocm` build number |
| `storage.storageClassName` | `openebs-hostpath` | Storage class |
