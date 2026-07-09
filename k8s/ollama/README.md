# ollama

[Ollama](https://ollama.com/) LLM inference server deployed on the dev cluster with AMD GPU (ROCm) support. Managed by ArgoCD. Open-WebUI is deployed as a companion application and connects to this service.

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
| External (browser) | `https://ollama.dev.butaco.net` |
| In-cluster (Open-WebUI) | `http://ollama.ollama.svc.cluster.local:11434` |

> `butaco.net` is a personal domain. Replace it in `values.yaml`.

## GPU

Requires one AMD GPU (`amd.com/gpu: "1"`). The node must be labeled `gpu: amd` and tainted `gpu=amd:NoSchedule`.

Uses the official `ollama/ollama:rocm` image.

## Storage

| PVC | Default Size | Mount Path |
|-----|-------------|------------|
| `ollama-models` | 100Gi | `/root/.ollama` |

## Key Values

| Key | Default | Description |
|-----|---------|-------------|
| `hostname` | `ollama.dev.butaco.net` | HTTPRoute hostname |
| `replicaCount` | `0` | Set to `1` to start (default off to save GPU) |
| `image.repository` | `ollama/ollama` | Ollama image |
| `image.tag` | `rocm` | ROCm-enabled image tag |
| `numCtx` | `32768` | Context window size (tokens) |
| `storage.size` | `100Gi` | PVC size for model storage |
| `storage.storageClassName` | `openebs-hostpath` | Storage class |

## Notes

- `replicaCount` defaults to `0` (scaled down when not in use). ArgoCD ignores replica drift via `ignoreDifferences`.
- Open-WebUI is deployed as a separate ArgoCD Application (rendered by the app-of-apps chart, enabled in `k8s/argocd/dev/apps-values.yaml`) using the upstream `open-webui` Helm chart with values from `k8s/open-webui/`.
