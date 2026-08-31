# lemonade-server

[Lemonade](https://github.com/lemonade-sdk/lemonade) Vulkan and ROCm inference on prd.

## Directory Structure

```
lemonade-server/
├── README.md
├── values.yaml           # Common Argo CD overrides
├── prd/
│   └── values.yaml       # prd-specific overrides
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
separate `lemonade-docker` repository. The current `b1319` bundles ROCm
`10.1.0a20260822`, one minor ahead of the ROCm 10.0 host stack. Rebuild and push
the image before bumping this tag, then re-verify GPU offload. ADR-0029's
benchmarks used `b1302` (ROCm 7.15) and need re-measuring.

### Switching backends

Both variants default to zero replicas and share PVCs. Always use gpu-switch so
only one GPU workload runs at a time:

```bash
# Choose one backend:
scripts/gpu-switch.sh lemonade-server        # Vulkan
# scripts/gpu-switch.sh lemonade-server-rocm # ROCm
scripts/gpu-switch.sh status
```

After verification, restore the normal workload:

```bash
scripts/gpu-switch.sh ollama
```

### Verifying GPU offload (Vulkan)

```bash
kubectl -n lemonade-server exec deploy/lemonade-server -- ./lemonade backends | grep -i llamacpp
# expect: llamacpp vulkan -> usable, no "Unsupported GPU"
kubectl -n lemonade-server logs deploy/lemonade-server | grep -iE "vulkan|RADV|gfx"
```

### Verifying GPU offload (ROCm variant)

Lemonade filters the ROCm `llama-server` child process output, so an empty log
grep does not prove CPU fallback. Verify device enumeration and VRAM usage
instead.

Device enumeration also confirms that the bundled ROCm userspace initialises
against the host KMD:

```bash
kubectl -n lemonade-server exec deploy/lemonade-server-rocm -c lemonade-server -- \
  /opt/llamacpp-rocm/llama-server --list-devices
```

Expect `ROCm0: AMD Radeon RX 9060 XT (16304 MiB, ...)`. If the list is empty,
check device injection and userspace/KMD compatibility; a recent image change
may require rolling `rocm.image.tag` back.

Then confirm tensors actually land in VRAM. Read `amd_gpu_used_vram` in
Prometheus before and after a load; idle sits at ~65 MiB, and a loaded model
moves it to weights + KV cache (Qwen3-0.6B Q4_0 at ctx 40960 reached 5195 MiB
on 2026-08-31, b1319). Confirm that the test model is downloaded, then trigger
the load:

```bash
kubectl -n lemonade-server exec deploy/lemonade-server-rocm -c lemonade-server -- \
  curl --fail-with-body -sS http://127.0.0.1:13305/api/v1/models/Qwen3-0.6B-GGUF
kubectl -n lemonade-server exec deploy/lemonade-server-rocm -c lemonade-server -- \
  curl --fail-with-body -sS http://127.0.0.1:13305/api/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"Qwen3-0.6B-GGUF","messages":[{"role":"user","content":"hi"}],"max_tokens":8}'
```

The spawned `llama-server` need not include `-ngl`: recent llama.cpp defaults to
automatic GPU-layer selection. Its absence alone is not a CPU-fallback signal.

## Storage

| PVC | Default Size | Mount Path |
|-----|-------------|------------|
| `lemonade-huggingface` | 90Gi | `/root/.cache/huggingface` |
| `lemonade-llama` | 5Gi | `/opt/lemonade/llama` |
| `lemonade-recipe` | 5Gi | `/root/.cache/lemonade` |

## Configuration

Argo CD applies values in this order, with later files taking precedence:

1. [`chart/values.yaml`](chart/values.yaml) — chart defaults
2. [`values.yaml`](values.yaml) — common overrides
3. [`prd/values.yaml`](prd/values.yaml) — prd overrides

Keep defaults in the values files rather than duplicating them here.
