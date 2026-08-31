# ollama

[Ollama](https://ollama.com/) ROCm inference on prd, consumed by Open WebUI and by
OpenCode ([ADR-0035](../../docs/adr/0035-opencode-connects-to-ollama.md);
`opencode.json` at the repo root).

## Directory Structure

```
ollama/
├── values.yaml           # Environment-level overrides (hostname, replicaCount)
└── chart/                # Custom Helm chart
    ├── Chart.yaml
    ├── values.yaml       # Default chart values
    └── templates/
        ├── deployment.yaml  # Recreate strategy; AMD GPU resource requests
        ├── pvc.yaml
        ├── service.yaml     # ClusterIP (also accessed by open-webui in-cluster)
        └── httproute.yaml   # HTTPRoute → shared-gateway-envoy
```

## Access

| Client | Address |
|--------|---------|
| External (browser) | `https://ollama.prd.butaco.net` |
| In-cluster (Open-WebUI) | `http://ollama.ollama.svc.cluster.local:11434` |

> `butaco.net` is a personal domain. Replace it in `values.yaml`.

## GPU / ROCm

Requests one `amd.com/gpu` on a `gpu=amd` labelled and tainted node.

The official `-rocm` image bundles userspace under `/usr/lib/ollama/rocm`; it
does not load host `/opt/rocm`:

| Layer | Managed in | Current |
|-------|-----------|---------|
| Kernel driver (KMD) | `ansible/roles/rocm` (`rocm_amdgpu_version`, `rocm_version`) | amdgpu 31.40 / ROCm 10.0 |
| Container userspace (UMD) | `chart/values.yaml` (`image.tag`) | ROCm 7.2.1, bundled in ollama 0.32.x |
| GPU target | host GPU / image build | `gfx1200` (RX 9060 XT) |

AMD's supported skew window covers bundled ROCm 7.2.1 with the 10.0 host
userspace; Ollama currently ships no newer backend. Verified working after the
7.14 -> 10.0 host upgrade: the bundled runtime reports
`library=ROCm compute=gfx1200 libdirs=ollama,rocm_v7_2` and offloads 100% to
the GPU.

gfx1200 is native; do not set `HSA_OVERRIDE_GFX_VERSION`.

### When the host ROCm version changes

After host driver upgrades, verify the pairing without automatically changing
the chart:

```bash
kubectl -n ollama scale deploy/ollama --replicas=1
kubectl -n ollama logs deploy/ollama | grep -i "rocm\|gfx\|amdgpu"
```

Logs must show gfx1200 and the ROCm library path; CPU fallback indicates an
incompatible pairing. Check upstream `ROCMVERSION` and `AMDGPU_TARGETS` on bumps.

## Storage

| PVC | Default Size | Mount Path |
|-----|-------------|------------|
| `ollama-models` | 100Gi | `/root/.ollama` |

## Key Values

| Key | Default | Description |
|-----|---------|-------------|
| `hostname` | `ollama.prd.butaco.net` | HTTPRoute hostname |
| `replicaCount` | `0` | Set to `1` to start (default off to save GPU) |
| `image.repository` | `ollama/ollama` | Ollama image |
| `image.tag` | `0.32.3-rocm` | ROCm-enabled image tag (bundles ROCm 7.2.1 userspace) |
| `numCtx` | `4096` (chart) / `131072` (`values.yaml`) | Context window size (tokens); 131072 is gemma4:12b's native max |
| `gateway.timeouts.request` | unset (chart) / `10m` (`values.yaml`) | End-to-end HTTPRoute timeout for long LLM responses |
| `gateway.timeouts.backendRequest` | unset (chart) / `10m` (`values.yaml`) | Gateway-to-Ollama request timeout |
| `storage.size` | `100Gi` | PVC size for model storage |
| `storage.storageClassName` | `openebs-hostpath` | Storage class |

## Notes

- `replicaCount: 0` leaves activation to gpu-switch; Argo CD ignores drift.
- The external route allows 10 minutes; in-cluster clients bypass Envoy.
- Open WebUI is a separate upstream-chart Application.
- `numCtx: 32768` / `kvCacheType: q8_0` (`values.yaml`) sizes the context window
  and KV cache for agent workloads (OpenCode), not just chat. Before running
  OpenCode, switch the GPU to this Deployment (`scripts/gpu-switch.sh ollama`)
  and pull the model it expects: `ollama pull gemma4:12b`.
