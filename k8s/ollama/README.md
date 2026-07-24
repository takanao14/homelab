# ollama

[Ollama](https://ollama.com/) LLM inference server deployed on the prd cluster with AMD GPU (ROCm) support. Managed by ArgoCD. Open-WebUI is deployed as a companion application and connects to this service.

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

Requires one AMD GPU (`amd.com/gpu: "1"`), allocated by the ROCm k8s-device-plugin deployed from `k0s/helmfile.yaml.gotmpl`. The node must be labeled `gpu: amd` and tainted `gpu=amd:NoSchedule`.

Uses the official `ollama/ollama:<version>-rocm` image. That image ships a **self-contained ROCm userspace** under `/usr/lib/ollama/rocm`; it never loads the host's `/opt/rocm`, so the container ROCm version and the host ROCm version are independent and only have to stay compatible:

| Layer | Managed in | Current |
|-------|-----------|---------|
| Kernel driver (KMD) | `ansible/roles/rocm` (`rocm_amdgpu_version`, `rocm_version`) | amdgpu 31.40 / ROCm 7.14 |
| Container userspace (UMD) | `chart/values.yaml` (`image.tag`) | ROCm 7.2.1, bundled in ollama 0.32.x |
| GPU target | host GPU / image build | `gfx1200` (RX 9060 XT) |

Since ROCm 6.4 AMD guarantees forward and backward compatibility between the amdgpu driver and ROCm userspace [up to a year apart](https://rocm.docs.amd.com/projects/install-on-linux/en/latest/reference/user-kernel-space-compat-matrix.html), so the bundled 7.2.1 userspace is a supported pairing with the ROCm 7.14 host driver. Upstream ollama has no ROCm 7.14 build; `rocm_v7_2` is the newest ROCm backend it ships.

`gfx1200` is included in the image's `AMDGPU_TARGETS`, so **no `HSA_OVERRIDE_GFX_VERSION` is needed** — setting it would break the natively supported target. Earlier gfx120x container bugs (missing `TensileLibrary_lazy_gfx120x.dat`, ollama [#12908](https://github.com/ollama/ollama/issues/12908) / [#12734](https://github.com/ollama/ollama/issues/12734)) were fixed upstream well before the pinned version.

### When the host ROCm version changes

`ansible/roles/rocm` moves independently of this chart. After a host ROCm/driver upgrade, only re-check the pairing — no chart change is normally required:

```bash
kubectl -n ollama scale deploy/ollama --replicas=1
kubectl -n ollama logs deploy/ollama | grep -i "rocm\|gfx\|amdgpu"
```

The log line should report the `gfx1200` device and the ROCm library path; a fallback to CPU means the driver/userspace pairing broke. When bumping `image.tag`, the bundled ROCm version is the `ROCMVERSION` arg in ollama's [Dockerfile](https://github.com/ollama/ollama/blob/main/Dockerfile) and the compiled targets are `AMDGPU_TARGETS` in [`llama/server/CMakePresets.json`](https://github.com/ollama/ollama/blob/main/llama/server/CMakePresets.json).

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
| `numCtx` | `4096` (chart) / `65536` (`values.yaml`) | Context window size (tokens) |
| `storage.size` | `100Gi` | PVC size for model storage |
| `storage.storageClassName` | `openebs-hostpath` | Storage class |

## Notes

- `replicaCount` defaults to `0` (scaled down when not in use). ArgoCD ignores replica drift via `ignoreDifferences`.
- Open-WebUI is deployed as a separate ArgoCD Application (rendered by the app-of-apps chart, enabled in `k8s/argocd/prd/apps-values.yaml`) using the upstream `open-webui` Helm chart with values from `k8s/open-webui/`.
